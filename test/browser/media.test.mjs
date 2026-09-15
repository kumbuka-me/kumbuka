import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

function image(id, filename) {
  return {
    id, filename, content_type: "image/png", size_bytes: 100,
    uploaded_by: 1, uploader: "User", created_at: "2026-09-01T00:00:00Z",
    usage_count: 0, url: `/media/${id}/image.png`,
  };
}

test("image pagination preserves its search and newer searches supersede pending requests", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome", headless: true,
  });
  let releaseSlow;
  const slowResponse = new Promise(resolve => { releaseSlow = resolve; });
  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const requests = [];
    await page.route("http://media.test/**", async route => {
      const url = new URL(route.request().url());
      if (url.pathname === "/api/images") {
        const query = url.searchParams.get("q");
        requests.push({ query, offset: url.searchParams.get("offset") });
        if (query === "slow") await slowResponse;
        await route.fulfill({ json: [image(2, `${query}.png`)] });
      } else if (url.pathname.startsWith("/assets/")) {
        await route.fulfill({
          body: await readFile(new URL(`../../web/dist/${url.pathname.slice(8)}`, import.meta.url)),
          contentType: "text/javascript",
        });
      } else if (url.pathname.startsWith("/media/")) {
        await route.fulfill({ status: 204 });
      } else {
        await route.fulfill({ contentType: "text/html", body: `
          <section data-media-settings-browser data-media-list-url="/api/images"
            data-media-page-size="1" data-media-offset="1">
            <form data-media-settings-search>
              <input data-media-settings-search-input value="original">
              <button>Search</button>
            </form>
            <a href="/" data-media-settings-clear>Clear</a>
            <div data-media-settings-list><article data-media-settings-item>original.png</article></div>
            <span data-media-settings-count></span>
            <button data-media-load-more>Load more</button>
            <span data-media-load-status></span>
          </section>
          <script type="module">
            import { initMedia } from '/assets/js/features/media.js';
            initMedia();
          </script>` });
      }
    });
    await page.goto("http://media.test/?image_q=original");
    const input = page.locator("input");
    await input.fill("unsubmitted");
    await page.getByText("Load more", { exact: true }).click();
    await page.waitForFunction(() => document.querySelectorAll("[data-media-settings-item]").length === 2);
    assert.deepEqual(requests, [{ query: "original", offset: "1" }]);
    assert.equal(new URL(page.url()).searchParams.get("image_q"), "original");

    await input.fill("slow");
    const pending = page.waitForRequest(url => url.url().includes("q=slow"));
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await pending;
    await input.fill("latest");
    await page.getByRole("button", { name: "Search", exact: true }).click();
    await page.waitForFunction(() => document.querySelector("[data-media-settings-list]").textContent.includes("latest.png"));
    releaseSlow();
    assert.equal(new URL(page.url()).searchParams.get("image_q"), "latest");
    assert.equal(await page.locator("[data-media-settings-item]").count(), 1);
    assert.equal(await page.locator("[data-media-load-more]").isEnabled(), true);
    assert.deepEqual(requests.slice(1), [
      { query: "slow", offset: "0" }, { query: "latest", offset: "0" },
    ]);
  } finally {
    releaseSlow();
    await browser.close();
  }
});
