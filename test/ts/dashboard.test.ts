import test from "node:test";
import assert from "node:assert/strict";

import { localDrafts } from "../../web/src/ts/features/dashboard.ts";

function storage(values: Record<string, string>) {
  const entries = Object.entries(values);
  return {
    length: entries.length,
    key(index: number) {
      return entries[index]?.[0] ?? null;
    },
    getItem(key: string) {
      return values[key] ?? null;
    },
  };
}

test("localDrafts returns Kumbuka editor drafts newest first", () => {
  const result = localDrafts(
    storage({
      unrelated: "ignored",
      "kumbuka.editor.draft:page:1": JSON.stringify({
        title: "Older",
        savedAt: 10,
        values: { original_slug: ["older"] },
      }),
      "kumbuka.editor.draft:page:2": JSON.stringify({
        title: "Newer",
        savedAt: 20,
        values: { original_slug: ["newer"] },
      }),
    }),
  );
  assert.deepEqual(
    result.map((draft) => draft.key),
    ["page:2", "page:1"],
  );
});

test("localDrafts ignores malformed values and supplies an untitled fallback", () => {
  const result = localDrafts(
    storage({
      "kumbuka.editor.draft:bad": "{",
      "kumbuka.editor.draft:no-time": JSON.stringify({
        title: "No time",
        values: {},
      }),
      "kumbuka.editor.draft:new": JSON.stringify({ savedAt: 42, values: {} }),
    }),
  );

  assert.equal(result.length, 1);
  assert.equal(result[0].title, "Untitled");
});

test("localDrafts keeps stable page keys and restores the original edit URL", () => {
  const result = localDrafts(
    storage({
      "kumbuka.editor.draft:page:42": JSON.stringify({
        title: "Private draft",
        savedAt: 100,
        values: { original_slug: ["guides/postgres"] },
      }),
    }),
  );

  assert.equal(result[0].key, "page:42");
  assert.equal(result[0].editURL, "/edit/guides/postgres");
});
