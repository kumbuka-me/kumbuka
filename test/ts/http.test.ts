import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

import {
  followedRedirectURL,
  requestJSON,
  responseProblem,
} from "../../web/src/ts/core/http.ts";

test("followedRedirectURL recognizes a redirect followed by fetch", () => {
  assert.equal(
    followedRedirectURL({
      redirected: true,
      url: "https://example.test/pages/new-page",
    }),
    "https://example.test/pages/new-page",
  );
  assert.equal(
    followedRedirectURL({
      redirected: false,
      url: "https://example.test/pages",
    }),
    null,
  );
});

void test("responseProblem validates problem payload fields", async () => {
  const response = new Response(JSON.stringify({ error: "fallback" }), {
    status: 400,
    headers: { "Content-Type": "application/json" },
  });

  const valid = await responseProblem(response, {
    error: "Invalid request",
    problems: { display_name: "Required" },
  });
  assert.equal(valid.message, "Invalid request\n\ndisplay name: Required");

  const invalid = await responseProblem(response, {
    error: false,
    problems: 42,
  });
  assert.equal(invalid.message, "HTTP 400");
});

void test("requestJSON returns unknown JSON and supplies an Accept header", async () => {
  const originalFetch = globalThis.fetch;
  let receivedAccept = "";

  globalThis.fetch = async (_input, init) => {
    receivedAccept = new Headers(init?.headers).get("Accept") ?? "";
    return new Response(JSON.stringify({ value: 7 }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  try {
    const payload = await requestJSON("https://example.test/api");

    assert.deepEqual(payload, { value: 7 });
    assert.equal(receivedAccept, "application/json");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

void test("requestJSON surfaces structured JSON errors", async () => {
  const originalFetch = globalThis.fetch;

  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        error: "Invalid request",
        problems: { title: "Required" },
      }),
      {
        status: 422,
        headers: { "Content-Type": "application/json" },
      },
    );

  try {
    let caught: unknown;

    try {
      await requestJSON("https://example.test/api");
    } catch (error) {
      caught = error;
    }

    assert.ok(caught instanceof Error);
    assert.equal(caught.message, "Invalid request\n\ntitle: Required");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

void test("Go problem fixtures are accepted by the browser", async () => {
  const fixtures = JSON.parse(
    readFileSync("test/contracts/http.json", "utf8"),
  ) as Record<string, unknown>;
  const response = new Response(null, { status: 404 });
  assert.equal(
    (await responseProblem(response, fixtures.problem_without_fields)).message,
    "Page not found.",
  );
  assert.equal(
    (await responseProblem(response, fixtures.problem_with_fields)).message,
    "Page validation failed.\n\ntitle: Title is required.",
  );
});

void test("optional malformed problems never hide a valid server message", async () => {
  const response = new Response(null, { status: 400 });
  for (const problems of [null, 42, [], { title: 7 }]) {
    assert.equal(
      (await responseProblem(response, { error: "Useful message", problems }))
        .message,
      "Useful message",
    );
  }
});

void test("requestJSON distinguishes no-content responses from malformed success bodies", async () => {
  const originalFetch = globalThis.fetch;
  try {
    globalThis.fetch = async () => new Response(null, { status: 204 });
    assert.equal(await requestJSON("https://example.test/api"), undefined);
    for (const body of ["", "not json", "<html>Sign in</html>"]) {
      globalThis.fetch = async () => new Response(body, { status: 200 });
      let caught: unknown;
      try {
        await requestJSON("https://example.test/api");
      } catch (error) {
        caught = error;
      }
      assert.ok(caught instanceof Error);
      assert.equal(caught.message, "Invalid JSON response.");
    }
  } finally {
    globalThis.fetch = originalFetch;
  }
});
