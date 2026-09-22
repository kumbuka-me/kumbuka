import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("visual widget source editing uses an application modal", async () => {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher, "the page template must include the mode switcher");

  const catalog = {
    pages: [],
    aliases: {},
    completions: [],
    inserts: [],
    widgets: [
      {
        id: "callout",
        name: "Callout",
        inline: false,
        syntax: { kind: "callout" },
        attributes: [
          {
            name: "kind",
            type: "enum",
            required: true,
            values: ["note", "info", "warning", "danger"],
          },
          {
            name: "body",
            type: "string",
            required: true,
            max_bytes: 16384,
          },
        ],
        settings: [
          { type: "select", label: "Type", attribute: "kind" },
          { type: "textarea", label: "Content", attribute: "body" },
        ],
        preview: {
          kind: "callout",
          callout: {
            class: "callout",
            body_class: "callout-body",
            kind_attribute: "kind",
            body_attribute: "body",
          },
        },
        plugin_id: "me.kumbuka.callouts",
      },
    ],
    widget_problems: [],
  };

  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });

  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    const dialogs = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("dialog", (dialog) => dialogs.push(dialog.type()));

    await page.route("http://source-dialog.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
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
          <form class="editor" data-editor-form>
            ${modeSwitcher}
            <div data-markdown-toolbar role="toolbar"></div>
            <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
              <div class="editor-source-pane">
                <textarea data-markdown-editor>!!! warning\nHelpful tip.</textarea>
              </div>
            </div>
          </form>
          <script type="module">
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initLazyVisualEditor();
          </script>`,
      });
    });

    await page.goto("http://source-dialog.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    await widget.click();

    const settings = page.getByRole("dialog", { name: "Edit Callout", exact: true });
    await settings.getByRole("button", { name: "Edit source" }).click();

    const sourceDialog = page.getByRole("dialog", {
      name: "Edit Callout source",
      exact: true,
    });
    const source = sourceDialog.getByLabel("Markdown source");
    await source.fill("not a callout");
    await sourceDialog.getByRole("button", { name: "Apply" }).click();
    assert.equal(
      await sourceDialog.getByRole("alert").textContent(),
      "Source is not valid Callout syntax.",
    );

    await source.fill("!!! info\nEdited source.");
    await sourceDialog.getByRole("button", { name: "Apply" }).click();
    assert.equal(
      await page.locator("textarea[data-markdown-editor]").inputValue(),
      "!!! info\nEdited source.",
    );
    assert.deepEqual(dialogs, [], "browser-native dialogs must not be used");
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
