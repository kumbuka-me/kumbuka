import test from "node:test";
import assert from "node:assert/strict";
import { webhookTemplateCompletions } from "../../web/src/ts/features/admin/webhooks.ts";

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
