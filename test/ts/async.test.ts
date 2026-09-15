import assert from "node:assert/strict";
import test from "node:test";

import {
  createDebouncer,
  createLatestRequest,
  isAbortError,
} from "../../web/src/ts/core/async.ts";

void test("createLatestRequest aborts the previous request", () => {
  const requests = createLatestRequest();
  const first = requests.next();
  const second = requests.next();

  assert.equal(first.aborted, true);
  assert.equal(second.aborted, false);
  assert.equal(requests.current(), second);

  requests.abort();

  assert.equal(second.aborted, true);
  assert.equal(requests.current(), undefined);
});

void test("createDebouncer runs only the latest scheduled callback", async () => {
  let calls = 0;
  const debouncer = createDebouncer(() => {
    calls += 1;
  }, 10);

  debouncer.schedule();
  debouncer.schedule();

  await new Promise<void>((resolve) => setTimeout(resolve, 25));
  assert.equal(calls, 1);

  debouncer.schedule();
  debouncer.cancel();

  await new Promise<void>((resolve) => setTimeout(resolve, 25));
  assert.equal(calls, 1);
});

void test("isAbortError recognizes aborted DOM operations", () => {
  assert.equal(isAbortError(new DOMException("Aborted", "AbortError")), true);
  assert.equal(isAbortError(new Error("other")), false);
});
