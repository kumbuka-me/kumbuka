import test from "node:test";
import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { chromium } from "playwright";

const pluginID = "me.kumbuka.external-files";
const completionModuleID = "source-completion";
const trigger = "{{external-file ";
const execFileAsync = promisify(execFile);

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

async function packagedExternalFileWidget() {
  const packagePath = fileURLToPath(
    new URL("../../plugins/external-files.kumbukaplugin", import.meta.url),
  );
  const { stdout } = await execFileAsync(
    "unzip",
    ["-p", packagePath, "assets/visual-editor.json"],
    { encoding: "utf8" },
  );
  const document = JSON.parse(stdout);
  assert.equal(document.version, 1);
  assert.equal(document.widgets.length, 1);
  const widget = document.widgets[0];
  const annotations = widget.settings.find(
    (setting) => setting.type === "table" && setting.attributes?.[0] === "note",
  );
  Object.assign(annotations, {
    row_separator: ":",
    columns: [
      { label: "Line(s)", type: "text" },
      { label: "Note", type: "textarea" },
    ],
  });
  Object.assign(widget.preview.card, {
    rendered: true,
    line_annotations: {
      attribute: "note",
      line_class: "external-file-line",
      line_number_class: "external-file-number",
    },
  });
  return widget;
}

async function externalFilesCatalog() {
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
    widgets: [{ plugin_id: pluginID, ...(await packagedExternalFileWidget()) }],
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
  const catalog = await externalFilesCatalog();
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
      if (path === "/api/preview") {
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({
            html: `<div class="external-file"><pre class="external-file-code"><code>
              <span class="external-file-line"><span class="external-file-number">1</span><span class="external-file-source">first</span></span>
              <span class="external-file-line"><span class="external-file-number">2</span><span class="external-file-source">second</span></span>
              <span class="external-file-line"><span class="external-file-number">3</span><span class="external-file-source">third</span></span>
              <span class="external-file-line"><span class="external-file-number">4</span><span class="external-file-source">fourth</span></span>
            </code></pre></div>`,
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
          <form class="editor" data-editor-form data-preview-url="/api/preview">
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

    await page
      .getByRole("button", { name: "External file", exact: true })
      .click();

    const picker = page.getByRole("dialog", { name: "Choose External file" });
    await picker.waitFor({ state: "visible" });
    assert.deepEqual(
      await picker
        .locator(".editor-completion-option strong")
        .allTextContents(),
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
      (
        await page.locator("textarea[data-markdown-editor]").inputValue()
      ).trimEnd(),
      '{{external-file source="engineering" path="cmd/server/main.go"}}',
    );

    const lineNumbers = widget.locator(".external-file-number");
    await lineNumbers.first().waitFor({ state: "visible" });
    await widget.evaluate((element) => {
      const sources = element.querySelectorAll(".external-file-source");
      const start = sources[1]?.firstChild;
      const end = sources[3]?.firstChild;
      if (!start || !end) throw new Error("preview source lines are missing");
      const range = document.createRange();
      range.setStart(start, 0);
      range.setEnd(end, end.textContent?.length || 0);
      const selection = window.getSelection();
      selection?.removeAllRanges();
      selection?.addRange(range);
      sources[3]?.dispatchEvent(new MouseEvent("mouseup", { bubbles: true }));
    });
    await widget.getByRole("button", { name: "Add note to lines 2–4" }).click();

    const annotationDialog = page.getByRole("dialog", {
      name: "Edit External file",
    });
    assert.equal(
      await annotationDialog.getByLabel("Line(s)").inputValue(),
      "2-4",
    );
    await annotationDialog
      .getByLabel("Note")
      .fill("These lines work together.");
    await annotationDialog.getByRole("button", { name: "Apply" }).click();

    assert.equal(
      (
        await page.locator("textarea[data-markdown-editor]").inputValue()
      ).trimEnd(),
      '{{external-file source="engineering" path="cmd/server/main.go" note="2-4:These lines work together."}}',
    );
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
