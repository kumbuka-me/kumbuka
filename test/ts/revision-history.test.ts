import test from "node:test";
import assert from "node:assert/strict";

import { revisionNumberFromURL } from "../../web/src/ts/features/revision-history.ts";

test("revision URLs select a specific positive revision", () => {
  assert.equal(revisionNumberFromURL("/revisions/guide?revision=27"), 27);
  assert.equal(
    revisionNumberFromURL("/revisions/guide?revision=3&source=audit"),
    3,
  );
});

test("revision URLs ignore missing and invalid selections", () => {
  for (const url of [
    "/revisions/guide",
    "/revisions/guide?revision=0",
    "/revisions/guide?revision=-2",
    "/revisions/guide?revision=3.5",
    "/revisions/guide?revision=Infinity",
    "/revisions/guide?revision=9007199254740992",
  ]) {
    assert.equal(revisionNumberFromURL(url), 0, url);
  }
});
