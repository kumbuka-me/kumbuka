import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("network macro fragments load after the page shell", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  let releaseFragment = () => {};
  try {
    const page = await browser.newPage();
    let fragmentRequested = false;
    const fragmentGate = new Promise((resolve) => {
      releaseFragment = resolve;
    });
    await page.route("http://deferred.test/**", async (route) => {
      const url = new URL(route.request().url());
      if (url.pathname.startsWith("/assets/")) {
        await route.fulfill({
          contentType: "text/javascript",
          body: await readFile(
            new URL("../../web/dist/" + url.pathname.slice(8), import.meta.url),
          ),
        });
        return;
      }
      if (url.pathname.startsWith("/api/plugin-fragments/")) {
        fragmentRequested = true;
        assert.equal(url.searchParams.get("v"), "1234");
        assert.equal(url.searchParams.get("locale"), "en");
        await fragmentGate;
        await route.fulfill({
          contentType: "text/html",
          body: '<div class="external-file">Loaded README</div>',
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<body data-current-page="guide" data-route-prefix=""><main><h1>Guide</h1><div class="kumbuka-deferred-fragment" role="status" aria-live="polite" data-kumbuka-deferred-plugin="me.kumbuka.external-files" data-kumbuka-deferred-module="external-files" data-kumbuka-deferred-index="0" data-kumbuka-deferred-version="1234" data-kumbuka-deferred-locale="en"><span class="kumbuka-deferred-spinner"></span><span>Loading external content…</span></div></main><script type="module" src="/assets/js/plugin-init.js"></script></body>`,
      });
    });

    const fragmentRequest = page.waitForRequest((request) =>
      request.url().includes("/api/plugin-fragments/"),
    );
    await page.goto("http://deferred.test/", { waitUntil: "domcontentloaded" });
    assert.equal(await page.getByRole("heading", { name: "Guide" }).isVisible(), true);
    const placeholder = page.locator(".kumbuka-deferred-fragment");
    assert.equal(await placeholder.isVisible(), true);
    assert.match(await placeholder.innerText(), /Loading external content/u);
    await fragmentRequest;
    assert.equal(fragmentRequested, true);
    releaseFragment();
    await page.getByText("Loaded README").waitFor();
    assert.equal(await page.locator(".kumbuka-deferred-spinner").count(), 0);
  } finally {
    releaseFragment?.();
    await browser.close();
  }
});
