import test from "node:test";
import assert from "node:assert/strict";
import { attachmentMarkdown } from "../../web/src/ts/features/editor/attachments.ts";

test("attachmentMarkdown inserts a normal Markdown file link", () => {
  assert.equal(
    attachmentMarkdown({
      filename: "values.yaml",
      url: "/attachments/12/values.yaml",
    }),
    "[values.yaml](/attachments/12/values.yaml)",
  );
});
