import test from "node:test";
import assert from "node:assert/strict";
import {
  sendWebhookTestRequest,
  webhookTemplateCompletions,
} from "../../web/src/ts/features/admin/webhooks.ts";

test("webhook template completions suggest payload fields", () => {
  const result = webhookTemplateCompletions("{{ .Pay", 7);

  assert.ok(result);
  assert.equal(result.start, 3);
  assert.equal(result.end, 7);
  assert.ok(result.items.some((item) => item.value === ".Payload.ObjectKey"));
});

test("webhook template completions filter nested recipient fields", () => {
  const value = "{{ .Payload.Rec";
  const result = webhookTemplateCompletions(value, value.length);

  assert.ok(result);
  assert.ok(result.items.length > 0);
  assert.ok(
    result.items.every((item) => item.value.startsWith(".Payload.Recipient")),
  );
});

test("webhook template completions ignore unrelated tokens", () => {
  assert.equal(webhookTemplateCompletions("{{ .Receiver", 12), null);
});

void test("webhook test request asks for JSON and accepts no-content success", async () => {
  const originalFetch = globalThis.fetch;
  let receivedAccept = "";
  let receivedBody = "";

  globalThis.fetch = async (_input, init) => {
    receivedAccept = new Headers(init?.headers).get("Accept") ?? "";
    receivedBody = String(init?.body ?? "");
    return new Response(null, { status: 204 });
  };

  try {
    const problem = await sendWebhookTestRequest(
      "https://example.test/admin/webhooks/9/test",
      new URLSearchParams({ event: "page.updated" }),
    );

    assert.equal(problem, null);
    assert.equal(receivedAccept, "application/json");
    assert.equal(receivedBody, "event=page.updated");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

void test("webhook test request returns structured server problems", async () => {
  const originalFetch = globalThis.fetch;

  globalThis.fetch = async () =>
    new Response(
      JSON.stringify({
        error: "Webhook test validation failed.",
        problems: { event: "Choose an event configured for this webhook." },
      }),
      {
        status: 422,
        headers: { "Content-Type": "application/json" },
      },
    );

  try {
    const problem = await sendWebhookTestRequest(
      "https://example.test/admin/webhooks/9/test",
      new URLSearchParams({ event: "page.updated" }),
    );

    assert.deepEqual(problem, {
      error: "Webhook test validation failed.",
      problems: { event: "Choose an event configured for this webhook." },
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
});

void test("webhook test request rejects unstructured failures", async () => {
  const originalFetch = globalThis.fetch;

  globalThis.fetch = async () =>
    new Response("upstream failed", { status: 502 });

  try {
    let caught: unknown;
    try {
      await sendWebhookTestRequest(
        "https://example.test/admin/webhooks/9/test",
        new URLSearchParams({ event: "page.updated" }),
      );
    } catch (error) {
      caught = error;
    }

    assert.ok(caught instanceof Error);
    assert.equal(caught.message, "HTTP 502");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
