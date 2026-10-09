import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("callout Markdown fields render and edit code blocks independently", async () => {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher);

  const catalog = {
    pages: [], aliases: {}, completions: [], completion_providers: [],
    inserts: [], widget_problems: [],
    widgets: [{
      id: "callout", name: "Callout", plugin_id: "me.kumbuka.callouts",
      inline: false, syntax: { kind: "callout" },
      attributes: [
        { name: "kind", type: "enum", required: true, values: ["info", "warning"] },
        { name: "body", type: "string", required: true, max_bytes: 16384 },
      ],
      settings: [
        { type: "select", label: "Type", attribute: "kind" },
        { type: "markdown", label: "Content", attribute: "body" },
      ],
      preview: {
        kind: "callout", callout: {
          class: "callout", body_class: "callout-body",
          kind_attribute: "kind", body_attribute: "body", body_format: "markdown",
        },
      },
    }],
  };
  const source = [
    "!!! info",
    "    send only for backups:",
    "    ```bash",
    "    --send-mail-any-operation=false",
    "    ```",
  ].join("\n");

  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("http://widget-markdown-field.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog) });
        return;
      }
      if (path === "/api/pages/preview") {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ html: '<aside class="callout info"><div class="callout-body">Preview</div></aside>' }),
        });
        return;
      }
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          body: await readFile(new URL(`../../web/dist/${path.slice(8)}`, import.meta.url)),
          contentType: path.endsWith(".css") ? "text/css" : "text/javascript",
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `
          <link rel="stylesheet" href="/assets/css/app.css">
          <form class="editor" data-editor-form data-preview-url="/api/pages/preview">
            ${modeSwitcher}
            <div data-markdown-toolbar role="toolbar"></div>
            <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
              <div class="editor-source-pane"><textarea data-markdown-editor>${source}</textarea></div>
            </div>
          </form>
          <script type="module">
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initLazyVisualEditor();
          </script>`,
      });
    });
    await page.goto("http://widget-markdown-field.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    await widget.click();
    const dialog = page.getByRole("dialog", { name: "Edit Callout" });
    const rich = dialog.locator(".visual-widget-markdown-content");
    await rich.locator("pre code").waitFor({ state: "visible" });
    assert.equal(await rich.locator("pre code").textContent(), "--send-mail-any-operation=false");
    assert.equal(await dialog.getByRole("button", { name: /Code language: Bash/ }).count(), 1);

    await dialog.getByRole("button", { name: "Markdown", exact: true }).click();
    const markdown = dialog.getByRole("textbox", { name: "Content Markdown source" });
    assert.match(await markdown.inputValue(), /```bash/);
    await dialog.getByRole("button", { name: "Visual", exact: true }).click();
    await rich.locator("pre code").waitFor({ state: "visible" });
    await dialog.getByRole("button", { name: "Apply" }).click();
    assert.equal((await page.locator("textarea[data-markdown-editor]").inputValue()).trimEnd(), source);

    await widget.click();
    const updated = page.getByRole("dialog", { name: "Edit Callout" });
    await updated.getByRole("button", { name: "Markdown", exact: true }).click();
    await updated.getByRole("textbox", { name: "Content Markdown source" }).fill(
      "**Updated**\n\n```bash\necho ok\n```",
    );
    await updated.getByRole("button", { name: "Visual", exact: true }).click();
    assert.equal(await updated.locator(".visual-widget-markdown-content strong").textContent(), "Updated");
    assert.equal(await updated.locator(".visual-widget-markdown-content pre code").textContent(), "echo ok");
    await updated.getByRole("button", { name: "Apply" }).click();
    assert.match(await page.locator("textarea[data-markdown-editor]").inputValue(), /    \*\*Updated\*\*/);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
