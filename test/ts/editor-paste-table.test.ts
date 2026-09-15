import test from "node:test";
import assert from "node:assert/strict";
import { tabularTextToMarkdown } from "../../web/src/ts/features/editor/paste-table.ts";

test("tabularTextToMarkdown converts spreadsheet TSV", () => {
  assert.equal(
    tabularTextToMarkdown("Service\tStatus\nAPI\tHealthy\nDB\tWarning"),
    "| Service | Status |\n| --- | --- |\n| API | Healthy |\n| DB | Warning |",
  );
});

test("tabularTextToMarkdown escapes markdown pipes", () => {
  assert.equal(
    tabularTextToMarkdown("Key\tValue\na\tb|c"),
    "| Key | Value |\n| --- | --- |\n| a | b\\|c |",
  );
});

test("tabularTextToMarkdown ignores normal pasted text", () => {
  assert.equal(tabularTextToMarkdown("one line"), null);
  assert.equal(tabularTextToMarkdown("first\nsecond"), null);
});
