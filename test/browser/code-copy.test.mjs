import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("rendered code blocks reveal copy without enhancing plugin diagrams", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    await page.route("http://code-copy.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          body: await readFile(
            new URL(`../../web/dist/${path.slice(8)}`, import.meta.url),
          ),
          contentType: path.endsWith(".css") ? "text/css" : "text/javascript",
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<link rel="stylesheet" href="/assets/css/app.css">
          <main class="prose">
            <div data-kumbuka-plugin="me.kumbuka.syntax-highlighting" data-kumbuka-module="highlight" data-kumbuka-code-block>
              <pre><code class="chroma">const answer = 42;</code></pre>
            </div>
            <div data-kumbuka-plugin="me.kumbuka.mermaid" data-kumbuka-module="diagram">
              <pre><code>graph LR; A --&gt; B</code></pre>
            </div>
          </main>
          <script type="module">
            import {setupMarkdownEnhancements} from '/assets/js/features/markdown.js';
            setupMarkdownEnhancements();
          </script>`,
      });
    });

    await page.goto("http://code-copy.test/");
    const highlighted = page.locator("[data-kumbuka-code-block]");
    const copy = highlighted.getByRole("button", {
      name: "Copy code to clipboard",
    });
    assert.equal(await copy.count(), 1);
    assert.equal(
      await copy.evaluate((element) => getComputedStyle(element).opacity),
      "0",
    );
    await highlighted.hover();
    await page.waitForFunction(
      (element) => getComputedStyle(element).opacity === "1",
      await copy.elementHandle(),
    );
    assert.equal(
      await page
        .locator('[data-kumbuka-plugin="me.kumbuka.mermaid"]')
        .getByRole("button", { name: "Copy code to clipboard" })
        .count(),
      0,
    );
  } finally {
    await browser.close();
  }
});
