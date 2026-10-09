import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("callout visual preview renders fenced Markdown without changing source", async () => {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher);

  const callouts = {
    version: 1,
    widgets: [{
      id: "callout", name: "Callout", inline: false,
      syntax: { kind: "callout" },
      attributes: [
        { name: "kind", type: "enum", required: true, values: ["note", "info", "danger"] },
        { name: "body", type: "string", required: true, max_bytes: 16384 },
      ],
      settings: [
        { type: "select", label: "Type", attribute: "kind" },
        { type: "textarea", label: "Content", attribute: "body" },
      ],
      preview: {
        kind: "callout",
        callout: {
          class: "callout", body_class: "callout-body",
          kind_attribute: "kind", body_attribute: "body", body_format: "markdown",
        },
      },
    }],
  };
  const catalog = {
    pages: [], aliases: {}, completions: [], completion_providers: [], inserts: [],
    widgets: [{ ...callouts.widgets[0], plugin_id: "me.kumbuka.callouts" }],
    widget_problems: [],
  };
  const markdown = [
    "!!! info",
    "    send only for backups:",
    "    ```bash",
    "    --send-mail-any-operation=false",
    "    ```",
  ].join("\n");
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome", headless: true,
  });

  try {
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.route("http://callout-markdown.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog) });
        return;
      }
      if (path === "/api/pages/preview") {
        const request = route.request().postDataJSON();
        const kind = /^!!! (\w+)/.exec(request.markdown)?.[1] || "note";
        const body = request.markdown.includes("**Updated**")
          ? "<p><strong>Updated</strong></p>"
          : '<p>send only for backups:</p><div class="code-block"><pre><code class="language-bash">--send-mail-any-operation=false</code></pre></div>';
        await route.fulfill({
          contentType: "application/json",
          body: JSON.stringify({ html: `<aside class="callout ${kind}"><div class="callout-body">${body}</div></aside>` }),
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
              <div class="editor-source-pane"><textarea data-markdown-editor>${markdown}</textarea></div>
            </div>
          </form>
          <script type="module">
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initLazyVisualEditor();
          </script>`,
      });
    });
    await page.goto("http://callout-markdown.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    const code = widget.locator(".callout pre code");
    await code.waitFor({ state: "visible" });
    assert.equal(await code.textContent(), "--send-mail-any-operation=false");
    assert.equal(await widget.locator(".callout-body").textContent(),
      "send only for backups:--send-mail-any-operation=false");
    assert.equal(await widget.locator(".callout-body").evaluate((el) =>
      getComputedStyle(el).whiteSpace), "normal");

    await widget.click();
    const dialog = page.getByRole("dialog", { name: "Edit Callout" });
    assert.match(await dialog.getByLabel("Content").inputValue(), /```bash/);
    await dialog.getByLabel("Type").selectOption("danger");
    await dialog.getByLabel("Content").fill("**Updated**");
    await widget.locator(".callout.danger strong").waitFor({ state: "visible" });
    // The visual Markdown serializer may add trailing block separators.
    assert.equal(
      (await page.locator("textarea[data-markdown-editor]").inputValue()).trimEnd(),
      markdown,
    );
    await dialog.getByRole("button", { name: "Apply" }).click();
    assert.match(await page.locator("textarea[data-markdown-editor]").inputValue(), /\*\*Updated\*\*/);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
