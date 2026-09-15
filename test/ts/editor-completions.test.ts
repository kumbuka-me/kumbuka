import test from "node:test";
import assert from "node:assert/strict";
import { completionTrigger } from "../../web/src/ts/features/editor/completions.ts";

test("completionTrigger opens the active plugin trigger", () => {
  assert.deepEqual(completionTrigger("Deploy to {{", 12, ["{{"]), {
    start: 10,
    query: "",
    trigger: "{{",
  });
  assert.deepEqual(completionTrigger("Deploy to {{env", 15, ["{{"]), {
    start: 10,
    query: "env",
    trigger: "{{",
  });
});

test("completionTrigger ignores completed braces and fenced code", () => {
  assert.equal(completionTrigger("{{environment}}", 15, ["{{"]), null);
  assert.equal(completionTrigger("```text\n{{env", 13, ["{{"]), null);
});
