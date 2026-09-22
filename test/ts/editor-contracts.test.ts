import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { parseEditorCatalog } from "../../web/src/ts/features/editor/catalog.ts";
import { editorDiagnostics } from "../../web/src/ts/features/editor/intelligence.ts";
import { normalizeServerDraft } from "../../web/src/ts/features/editor/experience.ts";

const fixtures = JSON.parse(
  readFileSync("test/contracts/http.json", "utf8"),
) as Record<string, unknown>;
const draft = JSON.parse(
  readFileSync("test/contracts/draft.json", "utf8"),
) as Record<string, unknown>;

test("editor accepts the empty Go catalog contract", () => {
  assert.deepEqual(parseEditorCatalog(fixtures.empty_catalog), {
    pages: [],
    aliases: {},
    completions: [],
    inserts: [],
    widgets: [],
    widget_problems: [],
  });
});

test("editor catalog checks every field used by both consumers", () => {
  const empty = {
    pages: [],
    aliases: {},
    completions: [],
    inserts: [],
    widgets: [],
    widget_problems: [],
  };
  for (const value of [
    { ...empty, aliases: [] },
    { ...empty, aliases: { old: 42 } },
    { ...empty, pages: [null] },
    { ...empty, pages: [{ slug: 42, title: "Invalid" }] },
    { ...empty, completions: null },
    { ...empty, inserts: null },
  ]) {
    let caught: unknown;
    try {
      parseEditorCatalog(value);
    } catch (error) {
      caught = error;
    }
    assert.ok(caught instanceof Error);
  }
});

test("Object prototype names are not catalog aliases", () => {
  const diagnostics = editorDiagnostics(
    "[[constructor]]",
    fixtures.empty_catalog,
  );
  assert.equal(
    diagnostics.some((item) => item.code === "broken-link"),
    true,
  );
});

test("editor accepts the empty Go draft contract", () => {
  const result = normalizeServerDraft(draft);
  assert.ok(result);
  assert.equal(result.pageID, 0);
  assert.deepEqual(result.values, {});
  assert.equal(result.savedAt, Date.parse("2026-09-06T12:00:00Z"));
});

test("editor rejects malformed draft fields rather than coercing them", () => {
  for (const change of [
    { page_id: "7" },
    { page_id: -1 },
    { page_id: 1.5 },
    { page_id: Number.MAX_SAFE_INTEGER + 1 },
    { title: {} },
    { slug: [] },
    { stale: "false" },
    { updated_at: "invalid" },
    { values: { title: null } },
    { values: { title: [7] } },
  ]) {
    assert.equal(normalizeServerDraft({ ...draft, ...change }), null);
  }
});

test("editor accepts declarative plugin action metadata", () => {
  const catalog = parseEditorCatalog({
    pages: [],
    aliases: {},
    completions: [],
    inserts: [
      {
        plugin_id: "me.kumbuka.strikethrough",
        module_id: "editor",
        name: "Strikethrough",
        markdown: "~~",
        suffix: "~~",
        placeholder: "strikethrough text",
        mode: "wrap",
        group: "text",
        icon: "strikethrough-lucide",
        inline: false,
      },
    ],
    widgets: [],
    widget_problems: [],
  });

  assert.equal(catalog.inserts[0]?.mode, "wrap");
  assert.equal(catalog.inserts[0]?.group, "text");
  assert.equal(catalog.inserts[0]?.suffix, "~~");
});
