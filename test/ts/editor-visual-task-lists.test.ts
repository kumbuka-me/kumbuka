import assert from "node:assert/strict";
import test from "node:test";
import {
  isVisualTaskListInsert,
  visualTaskListExtensions,
} from "../../web/src/ts/features/editor/visual-deps/list.ts";

test("visual editor replaces the stock listItem with a task-aware list item", () => {
  const extensions = visualTaskListExtensions();

  assert.deepEqual(
    extensions.map((extension) => extension.name),
    ["listItem"],
  );
});

test("visual editor recognizes declarative Markdown checklist actions", () => {
  assert.equal(
    isVisualTaskListInsert({ markdown: "- [ ] ", mode: "prefix-lines" }),
    true,
  );
  assert.equal(
    isVisualTaskListInsert({ markdown: "* [x] ", mode: "prefix-lines" }),
    true,
  );
  assert.equal(
    isVisualTaskListInsert({ markdown: "- ", mode: "prefix-lines" }),
    false,
  );
  assert.equal(
    isVisualTaskListInsert({ markdown: "- [ ] ", mode: "insert" }),
    false,
  );
});
