import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

const browserOptions = {
  channel: process.env.BROWSER_CHANNEL || "chrome",
  headless: true,
};

async function serveAsset(route) {
  const pathname = new URL(route.request().url()).pathname;
  if (!pathname.startsWith("/assets/")) return false;
  const source = new URL(`../../web/dist/${pathname.slice(8)}`, import.meta.url);
  const contentType = pathname.endsWith(".svg")
    ? "image/svg+xml"
    : pathname.endsWith(".css") ? "text/css" : "text/javascript";
  await route.fulfill({ contentType, body: await readFile(source) });
  return true;
}

async function openAdminFixture(page, body, initializer) {
  await page.route("http://admin-loading.test/**", async (route) => {
    if (await serveAsset(route)) return;
    await route.fulfill({
      contentType: "text/html",
      body: `<link rel="stylesheet" href="/assets/css/app.css"><body>
        ${body}
        <script type="module">
          ${initializer}
          document.documentElement.dataset.ready = "true";
        </script></body>`,
    });
  });
  await page.goto("http://admin-loading.test/");
  await page.locator("html[data-ready]").waitFor();
}

test("Check for updates displays the spinner while the form submits", async () => {
  const browser = await chromium.launch(browserOptions);
  try {
    const page = await browser.newPage();
    await page.addInitScript(() => {
      HTMLFormElement.prototype.submit = function () {
        document.documentElement.dataset.submissions = String(
          Number(document.documentElement.dataset.submissions || "0") + 1,
        );
      };
    });
    await openAdminFixture(page, `<form method="post" action="/admin/plugins/check-updates" data-plugin-update-check>
      <button type="submit" data-plugin-update-check-submit>
        <span class="kumbuka-spinner kumbuka-spinner--inline" data-plugin-update-check-spinner aria-hidden="true" hidden></span>
        <span data-plugin-update-check-label>Check for updates</span>
      </button></form>`, `
      import { initPluginUpdateProgress } from "/assets/js/features/admin/plugin_updates.js";
      initPluginUpdateProgress();`);
    await page.locator("form").evaluate((form) => form.requestSubmit());
    await page.locator("html[data-submissions='1']").waitFor();
    assert.equal(await page.locator("form").getAttribute("aria-busy"), "true");
    assert.equal(await page.locator("button").isDisabled(), true);
    assert.equal(await page.getByText("Checking for updates…").count(), 1);
    const spinner = page.locator("[data-plugin-update-check-spinner]");
    await spinner.waitFor({ state: "visible" });
    assert.match(await spinner.evaluate((node) => getComputedStyle(node).animationName), /kumbuka-spinner-rotate/u);
  } finally {
    await browser.close();
  }
});

test("Failed plugin update clears the spinner and restores the button", async () => {
  const browser = await chromium.launch(browserOptions);
  try {
    const page = await browser.newPage();
    await page.addInitScript(() => {
      window.fetch = () => new Promise((resolve) => {
        window.finishUpdate = () => resolve({
          ok: false,
          status: 500,
          redirected: false,
          text: async () => '<p role="alert">Catalog unavailable</p>',
        });
      });
    });
    await openAdminFixture(page, `<section class="settings-panel">
      <form action="/admin/plugins/me.kumbuka.mermaid/update" method="post" data-plugin-update>
        <button type="submit" data-plugin-update-submit>
          <span class="kumbuka-spinner kumbuka-spinner--inline" data-plugin-update-spinner aria-hidden="true" hidden></span>
          <span data-plugin-update-label>Update plugin</span>
        </button>
      </form></section>`, `
      import { initPluginUpdateProgress } from "/assets/js/features/admin/plugin_updates.js";
      initPluginUpdateProgress();`);
    await page.locator("form").evaluate((form) => form.requestSubmit());
    assert.equal(await page.locator("button").isDisabled(), true);
    await page.locator("[data-plugin-update-spinner]").waitFor({ state: "visible" });
    await page.evaluate(() => window.finishUpdate());
    await page.getByRole("alert").waitFor();
    assert.equal(await page.getByRole("alert").textContent(), "Catalog unavailable");
    assert.equal(await page.locator("[data-plugin-update-spinner]").isHidden(), true);
    assert.equal(await page.locator("button").isDisabled(), false);
    assert.equal(await page.locator("[data-plugin-update-label]").textContent(), "Update plugin");
  } finally {
    await browser.close();
  }
});

test("Page import uses the spinner and preserves progress feedback", async () => {
  const browser = await chromium.launch(browserOptions);
  try {
    const page = await browser.newPage();
    await page.addInitScript(() => {
      HTMLFormElement.prototype.submit = function () {
        document.documentElement.dataset.submitted = "true";
      };
    });
    await openAdminFixture(page, `<form data-import-form method="post" action="/admin/import">
      <button type="submit" data-import-submit>
        <span data-import-submit-icon>Upload</span>
        <span class="kumbuka-spinner kumbuka-spinner--inline" data-import-submit-spinner aria-hidden="true" hidden></span>
        <span data-import-submit-label>Import pages</span>
      </button>
      <span data-import-progress hidden role="status"></span>
    </form>`, `
      import { initAdminImport } from "/assets/js/features/admin/import.js";
      initAdminImport();`);
    await page.locator("form").evaluate((form) => form.requestSubmit());
    await page.locator("html[data-submitted]").waitFor();
    assert.equal(await page.locator("form").getAttribute("aria-busy"), "true");
    assert.equal(await page.locator("button").isDisabled(), true);
    assert.equal(await page.locator("[data-import-progress]").textContent(), "Importing pages…");
    assert.equal(await page.locator("[data-import-submit-label]").textContent(), "Importing…");
    await page.locator("[data-import-submit-spinner]").waitFor({ state: "visible" });
    assert.equal(await page.locator("[data-import-submit-icon]").isHidden(), true);
  } finally {
    await browser.close();
  }
});
