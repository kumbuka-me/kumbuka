import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import { block, pluginCatalog, pluginRoute } from "./plugin-fixture.mjs";

// Run with `make test-browser`.
// Use the real editor module and stylesheet with controlled preview latency.
test("split preview ignores cursor clicks and refreshes without flashing", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const requests = [];
    await page.route("http://preview.test/**", async (route) => {
      if (await pluginRoute(route, { fake: true })) return;
      const path = new URL(route.request().url()).pathname;
      if (path === "/preview") {
        const { markdown } = route.request().postDataJSON();
        requests.push(markdown);
        await new Promise((resolve) => setTimeout(resolve, 400));
        const escaped = markdown
          .replaceAll("&", "&amp;")
          .replaceAll("<", "&lt;");
        await route.fulfill({
          json: {
            html: `<p>${escaped}</p>${block}`,
          },
        });
      } else if (path.startsWith("/assets/")) {
        const asset = new URL(
          `../../web/dist/${path.slice(8)}`,
          import.meta.url,
        );
        await route.fulfill({
          body: await readFile(asset),
          contentType: path.endsWith(".css") ? "text/css" : "text/javascript",
        });
      } else {
        await route.fulfill({
          contentType: "text/html",
          body: `<link rel="stylesheet" href="/assets/css/app.css">
            ${pluginCatalog()}
            <form data-editor-form data-preview-url="/preview">
              <input name="slug" value="example">
              <button type="button" data-editor-mode="write">Markdown</button>
              <button type="button" data-editor-preview-toggle>Preview</button>
              <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
                <div class="editor-source-pane"><textarea class="editor-source" data-markdown-editor>Original</textarea></div>
                <section class="editor-preview" data-editor-preview hidden>
                  <div data-editor-preview-status></div>
                  <div class="prose editor-preview-prose" data-editor-preview-content></div>
                </section>
              </div>
            </form>
            <script type="module">
              import { initEditorPreview } from '/assets/js/features/editor/preview.js';
              initEditorPreview();
            </script>`,
        });
      }
    });
    await page.goto("http://preview.test/");
    await page.getByRole("button", { name: "Preview", exact: true }).click();
    const content = page.locator("[data-editor-preview-content]");
    await page.waitForFunction(
      () =>
        document.querySelector("[data-editor-preview-content] p")
          ?.textContent === "Original",
    );
    const originalY = (await content.boundingBox()).y;
    const source = page.locator("textarea");
    await source.click();
    await source.press("ArrowRight");
    await content.click();
    await page.waitForTimeout(700);
    assert.deepEqual(requests, ["Original"], "cursor movement must not render");

    await page.evaluate(() => {
      window.previewFrames = [];
      window.recordPreview = true;
      const record = () => {
        const content = document.querySelector("[data-editor-preview-content]");
        window.previewFrames.push({
          text: content.querySelector("p")?.textContent,
          y: content.getBoundingClientRect().y,
          statusHidden: document.querySelector("[data-editor-preview-status]")
            .hidden,
          diagramVisible: Boolean(
            content.querySelector("iframe[data-plugin-ready]"),
          ),
        });
        if (window.recordPreview) requestAnimationFrame(record);
      };
      requestAnimationFrame(record);
    });
    await source.fill("Intermediate");
    await page.waitForTimeout(250); // First request is in flight.
    await source.fill("Final");
    await page.waitForFunction(
      () =>
        document.querySelector("[data-editor-preview-content] p")
          ?.textContent === "Final",
    );
    const frames = await page.evaluate(() => {
      window.recordPreview = false;
      return window.previewFrames;
    });
    assert.ok(frames.length > 0);
    assert.ok(
      frames.every(
        (frame) =>
          frame.statusHidden && frame.diagramVisible && frame.y === originalY,
      ),
    );
    assert.ok(
      frames.every((frame) => ["Original", "Final"].includes(frame.text)),
      "never show empty or stale content",
    );
    assert.deepEqual(requests, ["Original", "Intermediate", "Final"]);

    await page.getByRole("button", { name: "Preview", exact: true }).click();
    await page.getByRole("button", { name: "Preview", exact: true }).click();
    await source.dispatchEvent("input");
    await page.waitForTimeout(700);
    assert.equal(requests.length, 3, "unchanged content should not refresh");
    assert.equal(await content.locator("p").textContent(), "Final");
  } finally {
    await browser.close();
  }
});
