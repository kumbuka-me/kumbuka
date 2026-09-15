import test from "node:test";
import assert from "node:assert/strict";

import { problemFieldLabel } from "../../web/src/ts/core/dialogs.ts";

test("problem field labels turn snake case into readable text", () => {
  assert.equal(problemFieldLabel("owner_group_id"), "Owner group id");
});

test("problem field labels turn kebab case into readable text", () => {
  assert.equal(problemFieldLabel("page-path"), "Page path");
});

test("problem field labels use a safe fallback for an empty name", () => {
  assert.equal(problemFieldLabel("  "), "Field");
});
