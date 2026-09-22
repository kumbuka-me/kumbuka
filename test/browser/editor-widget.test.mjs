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
    page.on("pageerror", (error) => errors.push(error.message));

    await page.route("http://widget.test/**", async (route) => {
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
                <textarea data-markdown-editor>{{status id="gaylvl1" set="custom1" options="gay1;gay2;gay3" colors="#64748b;#2563eb;#ca8a04" initial="gay1" prefix="Release"}}</textarea>
              </div>
            </div>
          </form>
          <script type="module">
            import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';
            initLazyVisualEditor();
          </script>`,
      });
    });

    await page.goto("http://widget.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    await widget.click();

    const dialog = page.getByRole("dialog", { name: "Edit Status" });
    const reusableSet = dialog.getByLabel("Reusable set");
    assert.equal(await reusableSet.inputValue(), "custom1");

    await dialog.getByRole("button", { name: "Add row" }).click();
    assert.equal(await reusableSet.inputValue(), "");
    const statuses = dialog.getByLabel("Status");
    assert.equal(await statuses.count(), 4);
    assert.equal(
      await statuses
        .last()
        .evaluate((element) => element === document.activeElement),
      true,
      "Add row must focus the new status field",
    );
    await statuses.last().fill("gay4");
    await dialog.getByRole("button", { name: "Apply" }).click();

    const source = page.locator("textarea[data-markdown-editor]");
    assert.match(await source.inputValue(), /options="gay1;gay2;gay3;gay4"/);
    assert.doesNotMatch(await source.inputValue(), /set="/);

    await widget.click();
    const nextDialog = page.getByRole("dialog", { name: "Edit Status" });
    const nextSet = nextDialog.getByLabel("Reusable set");
    await nextSet.fill("custom1");
    const nextStatuses = nextDialog.getByLabel("Status");
    assert.equal(await nextStatuses.count(), 1);
    assert.equal(await nextStatuses.first().inputValue(), "");
    await nextDialog.getByRole("button", { name: "Apply" }).click();

    assert.match(await source.inputValue(), /set="custom1"/);
    assert.doesNotMatch(await source.inputValue(), /options="/);
    assert.doesNotMatch(await source.inputValue(), /colors="/);
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});

test("callout widget previews type and content changes before apply", async () => {
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
            values: [
              "note",
              "info",
              "tip",
              "success",
              "warning",
              "danger",
              "error",
            ],
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
          {
            type: "textarea",
            label: "Content",
            attribute: "body",
            placeholder: "Important information.",
          },
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
    page.on("pageerror", (error) => errors.push(error.message));

    await page.route("http://callout.test/**", async (route) => {
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

    await page.goto("http://callout.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    const callout = widget.locator(".callout");
    await callout.waitFor({ state: "visible" });
    assert.equal(await callout.evaluate((element) => element.classList.contains("warning")), true);

    const source = page.locator("textarea[data-markdown-editor]");
    const originalSource = await source.inputValue();
    await widget.click();
    const dialog = page.getByRole("dialog", { name: "Edit Callout" });
    await dialog.getByLabel("Type").selectOption("danger");
    assert.equal(await callout.evaluate((element) => element.classList.contains("danger")), true);
    assert.equal(await callout.evaluate((element) => element.classList.contains("warning")), false);
    assert.equal(await source.inputValue(), originalSource, "live preview must not persist before Apply");

    await dialog.getByLabel("Content").fill("Stop now.");
    assert.equal(await callout.locator(".callout-body").textContent(), "Stop now.");
    await dialog.getByRole("button", { name: "Cancel" }).click();
    assert.equal(await callout.evaluate((element) => element.classList.contains("warning")), true);
    assert.equal(await callout.locator(".callout-body").textContent(), "Helpful tip.");
    assert.equal(await source.inputValue(), originalSource);

    await widget.click();
    const applyDialog = page.getByRole("dialog", { name: "Edit Callout" });
    await applyDialog.getByLabel("Type").selectOption("success");
    await applyDialog.getByLabel("Content").fill("Ready to ship.");
    await applyDialog.getByRole("button", { name: "Apply" }).click();

    assert.equal(await source.inputValue(), "!!! success\nReady to ship.");
    assert.equal(await callout.evaluate((element) => element.classList.contains("success")), true);
    assert.equal(await callout.locator(".callout-body").textContent(), "Ready to ship.");
    assert.deepEqual(errors, []);
  } finally {
    await browser.close();
  }
});
