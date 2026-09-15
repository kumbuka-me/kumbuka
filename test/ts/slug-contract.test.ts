import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { slugifyPagePath } from "../../web/src/ts/core/slug.ts";
import { editorSlug } from "../../web/src/ts/features/editor/intelligence.ts";
import { slugifyEditorPath } from "../../web/src/ts/features/editor/experience.ts";

const cases = JSON.parse(readFileSync("test/contracts/slugs.json", "utf8")) as {
  name: string;
  value: string;
  slug: string;
}[];

for (const entry of cases) {
  test(`page paths match Go: ${entry.name}`, () => {
    assert.equal(slugifyPagePath(entry.value), entry.slug);
    assert.equal(editorSlug(entry.value), entry.slug);
    assert.equal(slugifyEditorPath(entry.value), entry.slug);
  });
}
