import test from "node:test";
import assert from "node:assert/strict";

import {
  editorWordStats,
  preferredEditorMode,
  rememberEditorMode,
  resolvedGuidedEditorPath,
  slugifyEditorPath,
} from "../../web/src/ts/features/editor/experience.ts";
import { replaceAllPlainText } from "../../web/src/ts/features/editor/search.ts";
import { editorModeCopy } from "../../web/src/ts/features/editor/preview.ts";

test("editor reopens in the last source-visible mode, never preview", () => {
  const values = new Map<string, string>();
  const original = Object.getOwnPropertyDescriptor(globalThis, "localStorage");
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: {
      getItem: (key: string) => values.get(key) ?? null,
      setItem: (key: string, value: string) => values.set(key, value),
    },
  });

  try {
    assert.equal(preferredEditorMode(), "write");
    for (const mode of ["write", "split"] as const) {
      rememberEditorMode(mode);
      rememberEditorMode("preview");
      assert.equal(preferredEditorMode(), mode);
    }
    values.set("kumbuka.editor.mode", "preview");
    assert.equal(preferredEditorMode(), "write");
    values.set("kumbuka.editor.mode", "invalid");
    assert.equal(preferredEditorMode(), "write");
  } finally {
    if (original) Object.defineProperty(globalThis, "localStorage", original);
    else Reflect.deleteProperty(globalThis, "localStorage");
  }
});

test("editor path preview follows Kumbuka slug rules", () => {
  assert.equal(slugifyEditorPath("Postgres Restore"), "postgres-restore");
  assert.equal(
    slugifyEditorPath("infrastructure/Postgres Restore"),
    "infrastructure/postgres-restore",
  );
  assert.equal(slugifyEditorPath("  API & Database  "), "api-database");
});

test("guided editor paths combine the selected location and title", () => {
  assert.equal(
    resolvedGuidedEditorPath("Postgres Restore", "runbooks/database"),
    "runbooks/database/postgres-restore",
  );
  assert.equal(resolvedGuidedEditorPath("Top Level", ""), "top-level");
});

test("guided editor paths keep an existing final segment while moving", () => {
  assert.equal(
    resolvedGuidedEditorPath("Renamed title", "runbooks", "database-restore"),
    "runbooks/database-restore",
  );
});

test("editor word statistics handle empty and multiline Markdown", () => {
  assert.deepEqual(editorWordStats(""), {
    words: 0,
    characters: 0,
    lines: 1,
  });
  assert.deepEqual(editorWordStats("# Hello\n\nKumbuka wiki"), {
    words: 4,
    characters: 21,
    lines: 3,
  });
});

test("replace all supports case insensitive plain-text replacement", () => {
  assert.deepEqual(
    replaceAllPlainText("Kumbuka kumbuka KUMBUKA", "kumbuka", "Wiki"),
    {
      value: "Wiki Wiki Wiki",
      count: 3,
    },
  );
});

test("replace all can match case", () => {
  assert.deepEqual(
    replaceAllPlainText("Kumbuka kumbuka KUMBUKA", "Kumbuka", "Wiki", true),
    { value: "Wiki kumbuka KUMBUKA", count: 1 },
  );
});

test("editor mode copy matches the visible workspace", () => {
  assert.deepEqual(editorModeCopy("write"), {
    title: "Markdown",
    description: "Markdown stays the source of truth.",
  });
  assert.deepEqual(editorModeCopy("split"), {
    title: "Markdown & preview",
    description: "Edit Markdown with a live rendered preview.",
  });
  assert.deepEqual(editorModeCopy("preview"), {
    title: "Preview",
    description: "Rendered page preview.",
  });
});
