import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";

const checklistInsert = {
  pluginID: "me.kumbuka.task-lists",
  name: "Checklist",
  markdown: "- [ ] ",
  placeholder: "task",
  mode: "prefix-lines",
  inline: false,
};

async function editorPage(source) {
  const template = await readFile(
    new URL("../../web/src/templates/edit.gohtml", import.meta.url),
    "utf8",
  );
  const modeSwitcher = template
    .match(/<div class="editor-mode-switcher"[\s\S]*?<\/div>/)?.[0]
    .replace(/\{\{[\s\S]*?\}\}/g, "");
  assert.ok(modeSwitcher);

  return `<link rel="stylesheet" href="/assets/css/app.css"><form class="editor" data-editor-form>${modeSwitcher}<div data-markdown-toolbar role="toolbar"></div><div class="editor-workspace" data-editor-workspace data-editor-mode="write"><div class="editor-source-pane"><textarea data-markdown-editor>${source}</textarea></div></div></form><script type="module">import {initLazyVisualEditor} from '/assets/js/features/editor/visual-loader.js';initLazyVisualEditor();</script>`;
}

test("visual editor preserves mixed bullet and task items in one Markdown list", async () => {
  const source =
    "- regular\n- [ ] pending\n- [x] done\n- regular two\n\nparagraph";
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });

  try {
    const page = await browser.newPage();
    await page.route("http://task-list.test/**", async (route) => {
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
        body: await editorPage(source),
      });
    });

    await page.goto("http://task-list.test/");
    await page.getByRole("button", { name: "Visual", exact: true }).click();

    const topList = page.locator(".visual-editor-content > ul").first();
    await topList.waitFor({ state: "visible" });
    const items = topList.locator(":scope > li");
    assert.equal(await items.count(), 4);
    assert.deepEqual(
      await items.evaluateAll((rows) =>
        rows.map((row) => ({
          task: row.classList.contains("visual-task-item"),
          text: row.textContent?.trim(),
        })),
      ),
      [
        { task: false, text: "regular" },
        { task: true, text: "pending" },
        { task: true, text: "done" },
        { task: false, text: "regular two" },
      ],
    );

    const task = items.nth(1);
    assert.equal(
      await task.evaluate((element) => getComputedStyle(element).display),
      "flex",
    );
    assert.equal(
      await task.locator('input[type="checkbox"]').getAttribute("aria-label"),
      "Mark task complete",
    );

    await task.locator('input[type="checkbox"]').check();
    await page.waitForFunction(() => {
      const textarea = document.querySelector("[data-markdown-editor]");
      return (
        textarea instanceof HTMLTextAreaElement &&
        textarea.value.includes(
          "- regular\n- [x] pending\n- [x] done\n- regular two",
        )
      );
    });
    assert.equal(
      await task.locator('input[type="checkbox"]').getAttribute("aria-label"),
      "Mark task incomplete",
    );

    await items.first().click();
    await page.locator("form[data-editor-form]").evaluate((form, insert) => {
      form.dispatchEvent(
        new CustomEvent("editor:visual-command", {
          detail: { action: "plugin-insert", insert },
        }),
      );
    }, checklistInsert);

    await page.waitForFunction(() => {
      const textarea = document.querySelector("[data-markdown-editor]");
      return (
        textarea instanceof HTMLTextAreaElement &&
        textarea.value.startsWith("- [ ] regular\n- [x] pending")
      );
    });
    assert.equal(
      await items.first().evaluate((element) =>
        element.classList.contains("visual-task-item"),
      ),
      true,
    );
  } finally {
    await browser.close();
  }
});
