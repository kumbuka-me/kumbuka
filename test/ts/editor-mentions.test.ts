import test from "node:test";
import assert from "node:assert/strict";
import {
  mentionReplacement,
  mentionTrigger,
} from "../../web/src/ts/features/mentions.ts";

test("mentionTrigger opens on an empty or filtered mention", () => {
  assert.deepEqual(mentionTrigger("Hello @", 7), { start: 6, query: "" });
  assert.deepEqual(mentionTrigger("Hello @dan", 10), {
    start: 6,
    query: "dan",
  });
  assert.deepEqual(mentionTrigger("(@giotto", 8), {
    start: 1,
    query: "giotto",
  });
});

test("mentionTrigger does not treat email addresses as mentions", () => {
  assert.equal(mentionTrigger("mail@example", 12), null);
});

test("mentionTrigger ignores fenced and inline code", () => {
  assert.equal(mentionTrigger("```bash\necho @da", 16), null);
  assert.equal(mentionTrigger("Use `@dan", 9), null);
  assert.deepEqual(mentionTrigger("```\ncode\n```\n@da", 16), {
    start: 13,
    query: "da",
  });
});

test("mentionReplacement inserts the canonical username", () => {
  assert.equal(mentionReplacement("daniel"), "@daniel");
});
