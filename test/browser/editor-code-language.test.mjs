import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("visual code blocks choose a supported fenced language", async () => {
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
    page.on("pageerror", (error) => errors.push(error.message));

    await page.route("http://code-language.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            pages: [],
            aliases: {},
            completions: [],
            completion_providers: [],
            inserts: [],
            widgets: [],
            widget_problems: [],
          }),
        });
        return;
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
          <form class="editor" data-editor-form>
            ${modeSwitcher}
            <div data-markdown-toolbar role="toolbar">
              <button type="button" data-markdown-action="code-block">Code block</button>
            </div>
            <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
              <div class="editor-source-pane">
                <textarea data-markdown-editor>Text</textarea>
              </div>
            </div>
          </form>
          <script type="module">
            import {initMarkdownToolbar} from '/assets/js/features/editor/toolbar.js';
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initMarkdownToolbar(); initLazyVisualEditor();
          </script>`,
      });
    });

    await page.goto("http://code-language.test/");
    const source = page.locator("textarea[data-markdown-editor]");
    await source.fill("fmt.Println()\n");
    await source.selectText();
    await page.getByRole("button", { name: "Code block", exact: true }).click();
    const sourceDialog = page.getByRole("dialog", {
      name: "Code block language",
    });
    await sourceDialog.getByLabel("Search code languages").fill("golang");
    await sourceDialog.getByRole("option", { name: /^Go\b/ }).first().click();
    assert.match(await source.inputValue(), /^```go\nfmt\.Println\(\)\n\n```$/);

    await source.fill("Text");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    await page.getByRole("button", { name: "Code block", exact: true }).click();

    const dialog = page.getByRole("dialog", { name: "Code block language" });
    await dialog.getByLabel("Search code languages").fill("python");
    await dialog
      .getByRole("option", { name: /Python/ })
      .first()
      .click();

    const language = page.locator("button.visual-code-language");
    assert.equal(await language.textContent(), "Python");
    const block = page.locator("pre.visual-code-block");
    const copy = block.getByRole("button", { name: "Copy code to clipboard" });
    const languageBox = await language.boundingBox();
    const blockBox = await block.boundingBox();
    assert.ok(languageBox.x < blockBox.x + blockBox.width / 2);
    assert.equal(
      await copy.evaluate((element) => getComputedStyle(element).opacity),
      "0",
    );
    await block.hover();
    await page.waitForFunction(
      (element) => getComputedStyle(element).opacity === "1",
      await copy.elementHandle(),
    );
    assert.ok((await copy.boundingBox()).x > languageBox.x);
    assert.match(await source.inputValue(), /```python/);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
