import assert from "node:assert/strict";
import test from "node:test";
import { route } from "../../web/src/ts/core/route.ts";
import { requestJSON } from "../../web/src/ts/core/http.ts";

for (const prefix of ["", "/kumbuka"]) {
  test(`local routes preserve paths, queries, and fragments at ${prefix || "root"}`, () => {
    for (const target of [
      "/",
      "/pages/foo",
      "/api/search?q=a%20b#results",
      "/assets/v-test/js/main.js",
      "/plugins/test/digest/frames/main",
    ]) {
      assert.equal(route(target, prefix), prefix + target);
    }
    for (const target of [
      "//evil.test",
      "/../escape",
      "/pages/%2e%2e/escape",
      "/%5cevil",
      "/%2fevil",
      "/bad%zz",
    ]) {
      assert.equal(route(target, prefix), prefix + "/");
    }
    for (const target of [
      "https://example.test/a",
      "#section",
      "relative.png",
    ]) {
      assert.equal(route(target, prefix), target);
    }
  });
}

test("routes read the body prefix and generated API URLs reach fetch unchanged", async () => {
  const oldDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  const oldFetch = globalThis.fetch;
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { body: { dataset: { routePrefix: "/kumbuka" } } },
  });
  try {
    assert.equal(route("/pages/foo"), "/kumbuka/pages/foo");
    assert.equal(route("/pages/foo", "/pages"), "/pages/pages/foo");
    assert.equal(route("/kumbuka-other"), "/kumbuka/kumbuka-other");
    globalThis.fetch = async (input) => {
      assert.equal(input, "/kumbuka/api/search");
      return new Response("{}", { status: 200 });
    };
    await requestJSON(route("/api/search"));
  } finally {
    globalThis.fetch = oldFetch;
    if (oldDocument) Object.defineProperty(globalThis, "document", oldDocument);
    else Reflect.deleteProperty(globalThis, "document");
  }
});
