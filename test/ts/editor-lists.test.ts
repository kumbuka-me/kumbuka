import test from "node:test";
import assert from "node:assert/strict";

import { markdownListEnterEdit } from "../../web/src/ts/features/editor/lists.ts";

function applyEdit(
  value: string,
  caret: number,
): { value: string; caret: number } {
  const edit = markdownListEnterEdit(value, caret);
  assert.ok(edit);

  return {
    value: value.slice(0, edit.start) + edit.text + value.slice(edit.end),
    caret: edit.caret,
  };
}

test("Enter continues an unordered Markdown list", () => {
  const source = "- first item";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "- first item\n- ",
    caret: 15,
  });
});

test("Enter preserves plus and star bullet markers", () => {
  const plus = "+ first item";
  const star = "* first item";

  assert.deepEqual(applyEdit(plus, plus.length), {
    value: "+ first item\n+ ",
    caret: 15,
  });
  assert.deepEqual(applyEdit(star, star.length), {
    value: "* first item\n* ",
    caret: 15,
  });
});

test("Enter continues an unchecked task list with an unchecked task", () => {
  const source = "- [ ] first task";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "- [ ] first task\n- [ ] ",
    caret: 23,
  });
});

test("Enter after a completed task starts an unchecked task", () => {
  const source = "- [x] completed";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "- [x] completed\n- [ ] ",
    caret: 22,
  });
});

test("Enter preserves indentation for nested task lists", () => {
  const source = "  - [ ] nested";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "  - [ ] nested\n  - [ ] ",
    caret: 23,
  });
});

test("Enter preserves tab indentation", () => {
  const source = "\t- nested";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "\t- nested\n\t- ",
    caret: 13,
  });
});

test("Enter on an empty task marker exits the task list", () => {
  const source = "- [ ] first task\n- [ ] ";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "- [ ] first task\n",
    caret: 17,
  });
});

test("Enter on an empty bullet marker exits the list", () => {
  const source = "- first item\n- ";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "- first item\n",
    caret: 13,
  });
});

test("Enter increments ordered lists", () => {
  const source = "9. ninth";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "9. ninth\n10. ",
    caret: 13,
  });
});

test("Enter preserves ordered-list delimiters", () => {
  const source = "9) ninth";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "9) ninth\n10) ",
    caret: 13,
  });
});

test("list continuation does not run in the middle of a line", () => {
  const source = "- first item";

  assert.equal(markdownListEnterEdit(source, 7), null);
});

test("list continuation does not run inside backtick fenced code", () => {
  const source = "```text\n- literal item";

  assert.equal(markdownListEnterEdit(source, source.length), null);
});

test("list continuation does not run inside tilde fenced code", () => {
  const source = "~~~text\n- literal item";

  assert.equal(markdownListEnterEdit(source, source.length), null);
});

test("list continuation resumes after fenced code closes", () => {
  const source = "```text\n- literal item\n```\n- real item";

  assert.deepEqual(applyEdit(source, source.length), {
    value: "```text\n- literal item\n```\n- real item\n- ",
    caret: 41,
  });
});

test("Enter renumbers following ordered-list items", () => {
  const source = "1. first\n2. second\n3. third\n4. fourth";
  const caret = source.indexOf("\n3. third");

  assert.deepEqual(applyEdit(source, caret), {
    value: "1. first\n2. second\n3. \n4. third\n5. fourth",
    caret: caret + 4,
  });
});

test("Enter renumbers only siblings at the same indentation level", () => {
  const source = "1. first\n2. second\n   1. nested\n3. third";
  const caret = source.indexOf("\n   1. nested");

  assert.deepEqual(applyEdit(source, caret), {
    value: "1. first\n2. second\n3. \n   1. nested\n4. third",
    caret: caret + 4,
  });
});

test("Enter preserves closing-parenthesis markers while renumbering", () => {
  const source = "1) first\n2) second\n3) third";
  const caret = source.indexOf("\n3) third");

  assert.deepEqual(applyEdit(source, caret), {
    value: "1) first\n2) second\n3) \n4) third",
    caret: caret + 4,
  });
});

test("Enter stops renumbering when the ordered list ends", () => {
  const source = "1. first\n2. second\n3. third\nparagraph\n1. separate";
  const caret = source.indexOf("\n3. third");

  assert.deepEqual(applyEdit(source, caret), {
    value: "1. first\n2. second\n3. \n4. third\nparagraph\n1. separate",
    caret: caret + 4,
  });
});

test("Enter on an empty ordered marker restores following numbering", () => {
  const source = "1. first\n2. second\n3. \n4. third\n5. fourth";
  const caret = source.indexOf("\n4. third");

  assert.deepEqual(applyEdit(source, caret), {
    value: "1. first\n2. second\n\n3. third\n4. fourth",
    caret: source.indexOf("3. "),
  });
});
