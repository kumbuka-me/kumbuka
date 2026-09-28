import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

test("task assignee suggestions open as soon as @ is typed", async () => {
  const template = await readFile(new URL("../../web/src/templates/edit.gohtml", import.meta.url), "utf8");
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher);
  const task = {
    plugin_id: "me.kumbuka.tasks",
    id: "task",
    name: "Task",
    inline: false,
    syntax: { kind: "macro", name: "task" },
    attributes: [
      { name: "id", type: "identifier", required: true, max_bytes: 128 },
      { name: "text", type: "string", required: true, max_bytes: 512 },
      { name: "assignee", type: "string", max_bytes: 128 },
    ],
    settings: [
      { type: "text", label: "Task", attribute: "text" },
      { type: "text", label: "ID", attribute: "id" },
      { type: "mention", label: "Assignee", attribute: "assignee", placeholder: "@alice" },
    ],
    preview: {
      kind: "card",
      card: { class: "kumbuka-task", title: "Task", subtitle_attribute: "text", metadata_attributes: ["assignee"] },
    },
  };
  const catalog = { pages: [], aliases: {}, completions: [], completion_providers: [], inserts: [], widgets: [task], widget_problems: [] };
  const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || "chrome", headless: true });
  try {
    const page = await browser.newPage();
    await page.route("http://task-widget.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path === "/api/editor/catalog") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify(catalog) });
        return;
      }
      if (path === "/api/mentions/users") {
        await route.fulfill({ contentType: "application/json", body: JSON.stringify([{ username: "alice", display_name: "Alice Example", role: "editor" }]) });
        return;
      }
      if (path.startsWith("/assets/")) {
        await route.fulfill({ body: await readFile(new URL(`../../web/dist/${path.slice(8)}`, import.meta.url)), contentType: path.endsWith(".css") ? "text/css" : "text/javascript" });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<link rel="stylesheet" href="/assets/css/app.css"><form class="editor" data-editor-form>${modeSwitcher}<div data-markdown-toolbar role="toolbar"></div><div class="editor-workspace" data-editor-workspace data-editor-mode="write"><div class="editor-source-pane"><textarea data-markdown-editor>{{task id="one" text="Ship it"}}</textarea></div></div></form><script type="module">import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';initLazyVisualEditor();</script>`,
      });
    });

    await page.goto("http://task-widget.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();
    const widget = page.locator("[data-visual-widget]");
    await widget.waitFor({ state: "visible" });
    await widget.click();
    const assignee = page.getByRole("dialog", { name: "Edit Task" }).getByLabel("Assignee");
    await assignee.fill("@");
    const menu = page.getByRole("listbox", { name: "Mention a user" });
    await menu.waitFor({ state: "visible" });
    assert.equal(await menu.getByRole("option").count(), 1);
    assert.match(await menu.textContent(), /Alice Example/);
    await menu.getByRole("option").click();
    assert.equal(await assignee.inputValue(), "@alice");
  } finally {
    await browser.close();
  }
});
