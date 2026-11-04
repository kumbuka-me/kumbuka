import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("plugin-declared graph widgets own insertion and structured editing", async () => {
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

    await page.route("http://graph-widget.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        await route.fulfill({
          contentType: "application/json",
          body: await readFile(new URL("../contracts/graph-widget.json", import.meta.url), "utf8"),
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

    await page.goto("http://graph-widget.test/");
    const source = page.locator("textarea[data-markdown-editor]");
    await source.fill("");
    await page.getByRole("button", { name: "Code block", exact: true }).click();
    const language = page.getByRole("dialog", { name: "Code block language" });
    await language.getByLabel("Search code languages").fill("procedure");
    await language.getByRole("option", { name: "Procedure", exact: true }).click();
    assert.match(await source.inputValue(), /```procedure/);
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor();
    await widget.click();
    const settings = page.getByRole("dialog", { name: "Edit Procedure", exact: true });
    await settings.getByRole("button", { name: "Edit Procedure", exact: true }).click();
    const graph = page.getByRole("dialog", { name: "Procedure editor", exact: true });
    await graph.locator("fieldset").nth(2).getByLabel("Title", { exact: true }).fill("Contact Marco");
    assert.ok((await graph.getByRole("region", { name: "Graph preview" }).textContent()).includes("Yes → Contact Marco"));
    await graph.getByRole("button", { name: "Apply graph", exact: true }).click();
    await settings.getByRole("button", { name: "Apply", exact: true }).click();
    assert.match(await source.inputValue(), /action: Contact Marco/);
    await widget.click();
    await settings.getByRole("button", { name: "Edit Procedure", exact: true }).click();
    assert.equal(await graph.locator("fieldset").nth(2).getByLabel("Title", { exact: true }).inputValue(), "Contact Marco");
    await graph.getByRole("button", { name: "Cancel", exact: true }).click();
    await settings.getByRole("button", { name: "Cancel", exact: true }).click();
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
