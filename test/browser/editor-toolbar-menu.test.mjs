import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("plugin toolbar submenu children stay hidden until the menu opens", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    await page.route("http://toolbar.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/assets/css/app.css") {
        await route.fulfill({
          body: await readFile(
            new URL("../../web/dist/css/app.css", import.meta.url),
          ),
          contentType: "text/css",
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<link rel="stylesheet" href="/assets/css/app.css">
          <div class="markdown-toolbar" role="toolbar">
            <div class="markdown-toolbar-group"><span class="markdown-toolbar-group-label">Insert</span>
              <details class="markdown-toolbar-menu" data-toolbar-contribution="me.kumbuka.callouts:callout-menu">
                <summary aria-label="Callouts">Callouts</summary>
                <div class="markdown-toolbar-popover"><button type="button">Note</button><button type="button">Danger</button></div>
              </details>
            </div>
          </div>`,
      });
    });

    await page.goto("http://toolbar.test/");
    const menu = page.locator("details");
    const note = page.getByRole("button", { name: "Note", exact: true });
    assert.equal(await note.isVisible(), false);
    await menu.evaluate((element) => element.setAttribute("open", ""));
    assert.equal(await note.isVisible(), true);
    await menu.evaluate((element) => element.removeAttribute("open"));
    assert.equal(await note.isVisible(), false);
  } finally {
    await browser.close();
  }
});

test("heading menu previews the relative heading sizes", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    await page.route("http://heading-toolbar.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/assets/css/app.css") {
        await route.fulfill({
          body: await readFile(
            new URL("../../web/dist/css/app.css", import.meta.url),
          ),
          contentType: "text/css",
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<link rel="stylesheet" href="/assets/css/app.css">
          <div class="markdown-toolbar" role="toolbar">
            <details class="markdown-toolbar-menu markdown-toolbar-heading-menu" open>
              <summary>Heading</summary>
              <div class="markdown-toolbar-popover">
                <button type="button" data-markdown-action="heading-1"><span>Heading 1</span></button>
                <button type="button" data-markdown-action="heading-2"><span>Heading 2</span></button>
                <button type="button" data-markdown-action="heading-3"><span>Heading 3</span></button>
                <button type="button" data-markdown-action="heading-4"><span>Heading 4</span></button>
              </div>
            </details>
          </div>`,
      });
    });

    await page.goto("http://heading-toolbar.test/");
    const sizes = await page
      .locator(".markdown-toolbar-heading-menu button span")
      .evaluateAll((items) =>
        items.map((item) => Number.parseFloat(getComputedStyle(item).fontSize)),
      );
    assert.deepEqual(sizes, [24, 20, 17, 14]);
  } finally {
    await browser.close();
  }
});
