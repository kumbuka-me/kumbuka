import assert from "node:assert/strict";
import test from "node:test";

import { requireArrayOf } from "../../web/src/ts/core/guards.ts";

function isString(value: unknown): value is string {
  return typeof value === "string";
}

void test("requireArrayOf returns a fully valid array", () => {
  assert.deepEqual(requireArrayOf(["a", "b"], isString, "string list"), [
    "a",
    "b",
  ]);
});

void test("requireArrayOf rejects malformed entries instead of filtering them", () => {
  let caught: unknown;

  try {
    requireArrayOf(["a", 7], isString, "string list");
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Invalid string list.");
});
