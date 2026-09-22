import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("visual widget exactly-one fields switch modes and focus added rows", async () => {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher, "the page template must include the mode switcher");

  const catalog = JSON.parse(
    await readFile(
      new URL("../contracts/status-widget.json", import.meta.url),
      "utf8",
    ),
  );
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });

  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    let catalogRequests = 0;
    page.on("pageerror", (error) => errors.push(error.message));

    await page.route("http://widget.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        catalogRequests += 1;
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(catalog),
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
          <form class="editor" data-editor-form data-preview-url="/preview">
            ${modeSwitcher}
            <div data-markdown-toolbar role="toolbar"></div>
            <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
              <div class="editor-source-pane">
                <textarea data-markdown-editor>{{status id="release-check" set="custom1" options="Planned;Active;Done" colors="#64748b;#2563eb;#ca8a04" initial="Planned" prefix="Release" future="retained"}}</textarea>
              </div>
              <section data-editor-preview hidden><div data-editor-preview-status></div><div data-editor-preview-content></div></section>
            </div>
          </form>
          <script type="module">
            import {loadEditorCatalog} from '/assets/js/features/editor/catalog.js';
            void loadEditorCatalog();
            import {initEditorPreview} from '/assets/js/features/editor/preview.js';
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initLazyVisualEditor(); initEditorPreview();
          </script>`,
      });
    });

    await page.goto("http://widget.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    const source = page.locator("textarea[data-markdown-editor]");
    const original = await source.inputValue();
    await widget.focus();
    await widget.press("Enter");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Apply", exact: true })
      .click();
    assert.equal(
      await source.inputValue(),
      original,
      "unchanged forms must preserve the original source",
    );
    await widget.press("Enter");
    await page
      .getByRole("dialog")
      .getByLabel("ID", { exact: true })
      .press("Escape");
    assert.equal(await page.getByRole("dialog").count(), 0);
    assert.equal(
      await widget.evaluate((el) => el === document.activeElement),
      true,
    );
    await widget.click();

    const dialog = page.getByRole("dialog", { name: "Edit Status" });
    const reusableSet = dialog.getByLabel("Reusable set");
    assert.equal(await reusableSet.inputValue(), "custom1");

    await dialog.getByRole("button", { name: "Add row" }).click();
    assert.equal(await reusableSet.inputValue(), "");
    const statuses = dialog.getByLabel("Status", { exact: true });
    assert.equal(await statuses.count(), 4);
    assert.equal(
      await statuses
        .last()
        .evaluate((element) => element === document.activeElement),
      true,
      "Add row must focus the new status field",
    );
    await statuses.last().fill("Blocked");
    await dialog.getByRole("button", { name: "Apply" }).click();

    assert.match(
      await source.inputValue(),
      /options="Planned;Active;Done;Blocked"/,
    );
    assert.doesNotMatch(await source.inputValue(), /set="/);
    assert.match(await source.inputValue(), /future="retained"/);
    await page.locator(".tiptap").focus();
    await page.keyboard.press("ControlOrMeta+z");
    assert.match(await source.inputValue(), /options="Planned;Active;Done"/);
    await page.keyboard.press("ControlOrMeta+Shift+z");
    assert.match(
      await source.inputValue(),
      /options="Planned;Active;Done;Blocked"/,
    );

    await widget.click();
    const nextDialog = page.getByRole("dialog", { name: "Edit Status" });
    const nextSet = nextDialog.getByLabel("Reusable set");
    await nextSet.fill("custom1");
    const nextStatuses = nextDialog.getByLabel("Status", { exact: true });
    assert.equal(await nextStatuses.count(), 1);
    assert.equal(await nextStatuses.first().inputValue(), "");
    await nextDialog.getByRole("button", { name: "Apply" }).click();

    assert.match(await source.inputValue(), /set="custom1"/);
    assert.doesNotMatch(await source.inputValue(), /options="/);
    assert.doesNotMatch(await source.inputValue(), /colors="/);
    const saved = await source.inputValue();
    await widget.click();
    page.once("dialog", (dialog) => dialog.accept(saved + " trailing text"));
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "Edit source" })
      .click();
    assert.equal(await page.getByRole("alert").isVisible(), true);
    assert.equal(await source.inputValue(), saved);
    await page.getByRole("button", { name: "Markdown", exact: true }).click();
    assert.equal(
      await page.getByRole("dialog").count(),
      0,
      "switching modes closes widget settings",
    );
    assert.equal(
      catalogRequests,
      1,
      "the lazy bundle must share the existing editor catalog cache",
    );
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
