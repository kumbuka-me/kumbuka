import test from "node:test";
import assert from "node:assert/strict";
import {
  wikiLinkReplacement,
  wikiLinkTrigger,
} from "../../web/src/ts/features/editor/wikilinks.ts";

test("wikiLinkTrigger finds an unfinished wiki link", () => {
  assert.deepEqual(wikiLinkTrigger("See [[Post", 10), {
    start: 4,
    query: "Post",
  });
});

test("wikiLinkTrigger ignores closed or multiline links", () => {
  assert.equal(wikiLinkTrigger("[[Page]]", 8), null);
  assert.equal(wikiLinkTrigger("[[Page\nNext", 11), null);
});

test("wikiLinkReplacement inserts a normal wiki link", () => {
  assert.equal(
    wikiLinkReplacement("PostgreSQL restore"),
    "[[PostgreSQL restore]]",
  );
});
