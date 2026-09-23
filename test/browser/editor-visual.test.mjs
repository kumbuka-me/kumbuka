import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

for (const failFirst of [false, true])
  test(`lazy visual editor preserves editing (failed first download: ${failFirst})`, async () => {
    const template = await readFile(
      new URL("../../web/src/templates/edit.gohtml", import.meta.url),
      "utf8",
    );
    const modeSwitcher = template
      .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
      .replace(/\{\{[\s\S]*?\}\}/g, "");
    assert.ok(modeSwitcher, "the page template must include the mode switcher");
    const browser = await chromium.launch({
      channel: process.env.BROWSER_CHANNEL || "chrome",
      headless: true,
    });
    try {
      const page = await browser.newPage();
      page.setDefaultTimeout(10000);
      const errors = [];
      const chunks = [];
      let visualRequests = 0;
      let releaseVisual;
      const download = new Promise((resolve) => {
        releaseVisual = resolve;
      });
      page.on("pageerror", (error) => errors.push(error.message));
      await page.route("http://visual.test/**", async (route) => {
        const path = new URL(route.request().url()).pathname;
        if (path.includes("/editor/chunks/")) chunks.push(path);
        if (path === "/assets/js/features/editor/visual.js") {
          visualRequests += 1;
          if (failFirst && visualRequests === 1) {
            await route.fulfill({
              status: 503,
              body: "Temporarily unavailable",
            });
            return;
          }
          await download;
        }
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
          body: `
        <link rel="stylesheet" href="/assets/css/app.css">
        <form style="padding-top:180px" class="editor" data-editor-form data-preview-url="/preview">
          ${modeSwitcher}
          <div class="markdown-toolbar" data-markdown-toolbar role="toolbar">
            <button type="button" data-markdown-action="bold">Bold</button>
            <button type="button" data-markdown-action="italic">Italic</button>
            <button type="button" data-markdown-action="heading-4">Heading 4</button>
            <button type="button" data-markdown-action="quote">Quote</button>
            <button type="button" data-markdown-action="code-block">Code block</button>
            <button type="button" data-markdown-action="link">Link</button>
            <button type="button" data-plugin-insert-markdown="[[target|Target]]">Wiki link</button>
            <span data-table-insert-owner>
              <button type="button" data-table-format-open>Table</button>
              <span class="table-insert-popover" data-table-insert-popover hidden>
                <span data-table-insert-size></span><span data-table-insert-grid></span>
              </span>
            </span>
          </div>
          <div class="table-context-toolbar" data-table-context-toolbar hidden>
            <span data-table-context-label></span>
            <button type="button" data-table-action="insert-row-below">Add row</button>
            <select data-table-color-target><option value="header">Header</option><option value="cell">Cell</option></select>
            <button type="button" data-table-tone="blue">Blue</button>
            <input type="checkbox" data-table-format-sortable>
            <input type="checkbox" data-table-format-filterable>
            <details data-table-context-more></details>
          </div>
          <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
            <div class="editor-source-pane"><textarea data-markdown-editor>Original</textarea></div>
            <section data-editor-preview hidden><div data-editor-preview-status></div><div data-editor-preview-content></div></section>
          </div>
        </form>
        <script type="module">
          import { initMarkdownToolbar, insertMarkdownAtSelection } from '/assets/js/features/editor/toolbar.js';
          import { initTablePalette } from '/assets/js/features/editor/tables.js';
          import { initLazyVisualEditor } from '/assets/js/features/editor/visual-loader.js';
          import { initEditorPreview } from '/assets/js/features/editor/preview.js';
          window.insertMarkdown = markdown => insertMarkdownAtSelection(document.querySelector('textarea'), markdown);
          initMarkdownToolbar(); initTablePalette(); initLazyVisualEditor(); initEditorPreview();
        </script>`,
        });
      });
      await page.goto("http://visual.test/");
      const toolbar = page.getByRole("toolbar");
      const initial = await toolbar.boundingBox();
      assert.deepEqual(
        chunks,
        [],
        "Markdown must not download visual dependency chunks",
      );
      assert.equal(visualRequests, 0, "Markdown must not download Tiptap");
      const source = page.locator("textarea");
      await source.selectText();
      await page.getByRole("button", { name: "Bold", exact: true }).click();
      assert.equal(await source.inputValue(), "**Original**");
      await page.getByRole("button", { name: "Visual", exact: true }).click();
      if (failFirst)
        await page.getByRole("button", { name: "Retry", exact: true }).click();
      await page.getByText("Loading visual editor…", { exact: true }).waitFor();
      await page.getByRole("button", { name: "Markdown", exact: true }).click();
      assert.equal(await toolbar.evaluate((el) => el.inert), false);
      releaseVisual();
      await page.waitForResponse((response) =>
        new URL(response.url()).pathname.endsWith("/editor/visual.js"),
      );
      assert.equal(
        await page.locator("[data-visual-editor-pane]").isVisible(),
        false,
      );
      await page.getByRole("button", { name: "Visual", exact: true }).click();
      const visual = page.locator(".tiptap");
      await visual.waitFor({ state: "visible" });
      assert.equal(await page.getByRole("toolbar").count(), 1);
      const active = await toolbar.boundingBox();
      assert.equal(active.width, initial.width);
      assert.equal(active.height, initial.height);
      await visual.selectText();
      await page.getByRole("button", { name: "Italic", exact: true }).click();
      assert.equal(
        await visual.locator("strong em, em strong").textContent(),
        "Original",
      );
      await page
        .getByRole("button", { name: "Heading 4", exact: true })
        .click();
      assert.equal(await visual.locator("h4").textContent(), "Original");
      await page.getByRole("button", { name: "Markdown", exact: true }).click();
      assert.match(await source.inputValue(), /^#### /);
      assert.equal(
        await toolbar.locator("[data-markdown-action].active").count(),
        0,
      );
      await source.fill("First\n\nLast");
      await page.getByRole("button", { name: "Visual", exact: true }).click();
      await visual.locator("p").first().click();
      await page.keyboard.press("Home");
      await page
        .getByRole("button", { name: "Wiki link", exact: true })
        .click();
      assert.equal(
        await visual.locator("[data-kumbuka-inline] > span").textContent(),
        "Target",
      );
      await page.evaluate(() => window.insertMarkdown("![image](/image.png)"));
      assert.equal(
        await visual.locator("img[src]").getAttribute("src"),
        "/image.png",
      );
      assert.match(await source.inputValue(), /\[\[target\|Target\]\]/);
      assert.match(await source.inputValue(), /!\[image\]\(\/image.png\)/);
      await visual.selectText();
      await page.getByRole("button", { name: "Quote", exact: true }).click();
      assert.ok(await visual.locator("blockquote").count());
      await page.getByRole("button", { name: "Quote", exact: true }).click();
      await page
        .getByRole("button", { name: "Code block", exact: true })
        .click();
      await page
        .getByRole("dialog", { name: "Code block language" })
        .getByRole("option", { name: /Plain text/ })
        .click();
      assert.ok(await visual.locator("pre").count());
      await page.getByRole("button", { name: "Markdown", exact: true }).click();
      await source.fill("Text");
      await page.getByRole("button", { name: "Visual", exact: true }).click();
      await visual.locator("p").click();
      await page.keyboard.press("End");
      page.once("dialog", (dialog) => dialog.accept("https://example.com"));
      await page.getByRole("button", { name: "Link", exact: true }).click();
      assert.equal(
        await visual.locator("a").textContent(),
        "https://example.com",
      );
      assert.match(await source.inputValue(), /https:\/\/example.com/);
      await page.getByRole("button", { name: "Table", exact: true }).click();
      await page
        .getByRole("gridcell", {
          name: "2 body rows by 3 columns",
          exact: true,
        })
        .click();
      assert.equal(await visual.locator("tr").count(), 3);
      assert.equal(await visual.locator("th").count(), 3);
      await page.locator("[data-table-color-target]").selectOption("header");
      await page.getByRole("button", { name: "Blue", exact: true }).click();
      await page.waitForFunction(() =>
        document.querySelector(".tiptap th.table-tone-blue"),
      );
      assert.match(await source.inputValue(), /header=blue/);
      const options = visual.locator("[data-visual-table-options]");
      assert.equal(await options.count(), 1);
      assert.equal(
        await options.isVisible(),
        false,
        "table metadata must not appear as visual content",
      );
      await visual.locator("td").first().click();
      await page.getByRole("button", { name: "Add row", exact: true }).click();
      assert.equal(await visual.locator("tr").count(), 4);
      const surfaceBox = await page
        .locator("[data-visual-editor]")
        .boundingBox();
      const paragraphBox = await visual.locator("p").first().boundingBox();
      assert.ok(
        paragraphBox.x - surfaceBox.x < 25,
        "visual content should align near the pane edge",
      );
      const firstCell = visual.locator("th").first();
      assert.ok(
        (await firstCell.boundingBox()).height < 45,
        "empty cells should be compact",
      );
      await visual.locator("td").first().click();
      assert.equal(
        await page.locator("[data-table-color-target]").inputValue(),
        "cell",
      );
      await page.getByRole("button", { name: "Blue", exact: true }).click();
      assert.match(await source.inputValue(), /cell:1,1=blue/);
      const colored = visual.locator("td").first();
      assert.notEqual(
        await colored.evaluate((el) => getComputedStyle(el).backgroundColor),
        "rgba(0, 0, 0, 0)",
      );
      async function resizeRow(delta) {
        const handle = firstCell.locator(".row-resize-handle");
        const box = await handle.boundingBox();
        await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
        await page.mouse.down();
        await page.mouse.move(
          box.x + box.width / 2,
          box.y + box.height / 2 + delta,
          { steps: 5 },
        );
        await page.mouse.up();
      }
      const compact = (await firstCell.boundingBox()).height;
      await resizeRow(50);
      const enlarged = (await firstCell.boundingBox()).height;
      assert.ok(enlarged > compact + 35);
      await resizeRow(-30);
      assert.ok((await firstCell.boundingBox()).height < enlarged - 20);
      assert.match(await source.inputValue(), /heights=/);
      const cellBox = await firstCell.boundingBox();
      await page.mouse.move(cellBox.x + cellBox.width - 2, cellBox.y + 10);
      await page.mouse.down();
      await page.mouse.move(cellBox.x + cellBox.width + 45, cellBox.y + 10, {
        steps: 5,
      });
      await page.mouse.up();
      assert.match(await source.inputValue(), /widths=/);
      const wider = await firstCell.boundingBox();
      await page.mouse.move(wider.x + wider.width - 2, wider.y + 10);
      await page.mouse.down();
      await page.mouse.move(wider.x + wider.width - 32, wider.y + 10, {
        steps: 5,
      });
      await page.mouse.up();
      assert.ok((await firstCell.boundingBox()).width < wider.width - 20);
      const saved = await source.inputValue();
      const resizedWidth = (await firstCell.boundingBox()).width;
      const resizedHeight = (await firstCell.boundingBox()).height;
      await page.getByRole("button", { name: "Markdown", exact: true }).click();
      await source.fill(saved + "\n");
      await page.getByRole("button", { name: "Visual", exact: true }).click();
      assert.ok(
        Math.abs((await firstCell.boundingBox()).height - resizedHeight) < 3,
      );
      assert.equal(await options.isVisible(), false);
      assert.match(await source.inputValue(), /cell:1,1=blue/);
      assert.match(await firstCell.getAttribute("colwidth"), /\d+/);
      assert.ok(
        Math.abs((await firstCell.boundingBox()).width - resizedWidth) < 3,
      );
      assert.equal(
        visualRequests,
        failFirst ? 2 : 1,
        "mode switches reuse the loaded editor module",
      );
      assert.ok(chunks.length > 1, "Visual loads split dependency chunks");
      assert.equal(
        new Set(chunks).size,
        chunks.length,
        "mode switches reuse cached chunks",
      );
      assert.deepEqual(errors, []);
    } finally {
      await browser.close();
    }
  });
