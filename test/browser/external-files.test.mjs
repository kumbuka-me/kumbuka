import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

const pluginID = "me.kumbuka.external-files";
const completionModuleID = "source-completion";
const trigger = "{{external-file ";

function sourceCompletion(name, repository) {
  return {
    plugin_id: pluginID,
    module_id: completionModuleID,
    trigger,
    label: name,
    detail: repository,
    replacement: `{{external-file source="${name}" path="README.md"}}`,
  };
}

function externalFilesCatalog() {
  return {
    pages: [],
    aliases: {},
    completions: [
      sourceCompletion("engineering", "acme/platform"),
      sourceCompletion("documentation", "acme/docs"),
    ],
    completion_providers: [
      {
        plugin_id: pluginID,
        module_id: completionModuleID,
        resource_id: "sources",
        resource_name: "Sources",
        trigger,
        replacement: '{{external-file source="${name}" path="README.md"}}',
        label_field: "name",
        detail_field: "repository",
        fields: [],
        can_create: false,
      },
    ],
    inserts: [
      {
        plugin_id: pluginID,
        module_id: "editor",
        name: "External file",
        description: "Display a file from a configured source.",
        markdown: trigger,
        mode: "insert",
        group: "insert",
        icon: "file-code-2-lucide",
        inline: false,
      },
    ],
    widgets: [
      {
        plugin_id: pluginID,
        id: "external-file",
        name: "External file",
        inline: false,
        syntax: { kind: "macro", name: "external-file" },
        attributes: [
          { name: "source", type: "string", required: true, max_bytes: 128 },
          { name: "path", type: "string", required: true, max_bytes: 1024 },
        ],
        settings: [
          {
            type: "resource",
            label: "Source",
            attribute: "source",
            placeholder: "Choose a configured source…",
            completion_module_id: completionModuleID,
          },
          {
            type: "text",
            label: "Path",
            attribute: "path",
            placeholder: "src/main.go",
          },
        ],
        preview: {
          kind: "card",
          card: {
            class: "external-file",
            title: "External file",
            subtitle_attribute: "path",
            metadata_attributes: ["source"],
            body_text: "Repository content is loaded when the page is rendered.",
          },
        },
      },
    ],
    widget_problems: [],
  };
}

async function editorModeSwitcher() {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher, "the page template must include the mode switcher");
  return modeSwitcher;
}

test("External Files insert and widget list configured sources", async () => {
  const modeSwitcher = await editorModeSwitcher();
  const catalog = externalFilesCatalog();
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });

  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));

    await page.route("http://external-files.test/**", async (route) => {
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
            <div data-markdown-toolbar role="toolbar">
              <button type="button"
                      data-plugin-insert-plugin-id="${pluginID}"
                      data-plugin-insert-name="External file"
                      data-plugin-insert-markdown="{{external-file "
                      data-plugin-insert-mode="insert"
                      data-plugin-insert-inline="false"
                      title="Display a file from a configured source."
                      aria-label="External file">External file</button>
            </div>
            <div class="editor-workspace" data-editor-workspace data-editor-mode="write">
              <div class="editor-source-pane">
                <textarea data-markdown-editor></textarea>
              </div>
            </div>
          </form>
          <script type="module">
            import {initMarkdownToolbar} from '/assets/js/features/editor/toolbar.js';
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initMarkdownToolbar();
            initLazyVisualEditor();
          </script>`,
      });
    });

    await page.goto("http://external-files.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    await page.locator("[data-visual-editor] .ProseMirror").waitFor({
      state: "visible",
    });

    await page.getByRole("button", { name: "External file", exact: true }).click();

    const picker = page.getByRole("dialog", { name: "Choose External file" });
    await picker.waitFor({ state: "visible" });
    assert.deepEqual(
      await picker.locator(".editor-completion-option strong").allTextContents(),
      ["engineering", "documentation"],
    );
    assert.deepEqual(
      await picker.locator(".editor-completion-option small").allTextContents(),
      ["acme/platform", "acme/docs"],
    );

    await picker.getByRole("button", { name: /documentation/u }).click();

    const widget = page.locator(
      '[data-visual-widget][data-plugin-id="me.kumbuka.external-files"]',
    );
    await widget.waitFor({ state: "visible" });
    await widget.click();

    const settings = page.getByRole("dialog", { name: "Edit External file" });
    const source = settings.getByLabel("Source");
    assert.deepEqual(await source.locator("option").allTextContents(), [
      "engineering — acme/platform",
      "documentation — acme/docs",
    ]);
    assert.equal(await source.inputValue(), "documentation");

    await source.selectOption("engineering");
    await settings.getByLabel("Path").fill("cmd/server/main.go");
    await settings.getByRole("button", { name: "Apply" }).click();

    assert.equal(
      await page.locator("textarea[data-markdown-editor]").inputValue(),
      '{{external-file source="engineering" path="cmd/server/main.go"}}',
    );
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
