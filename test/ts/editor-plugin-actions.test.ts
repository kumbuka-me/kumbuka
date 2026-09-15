import test from "node:test";
import assert from "node:assert/strict";
import { applyEditorInsertAction } from "../../web/src/ts/features/editor/toolbar.ts";

function textarea(
  value: string,
  selectionStart: number,
  selectionEnd = selectionStart,
): HTMLTextAreaElement {
  const editor = {
    value,
    selectionStart,
    selectionEnd,
    scrollTop: 0,
    setRangeText(
      replacement: string,
      start: number,
      end: number,
      mode: SelectionMode = "preserve",
    ) {
      this.value =
        this.value.slice(0, start) + replacement + this.value.slice(end);
      const after = start + replacement.length;
      if (mode === "select") {
        this.selectionStart = start;
        this.selectionEnd = after;
      } else {
        this.selectionStart = after;
        this.selectionEnd = after;
      }
    },
    setSelectionRange(start: number, end: number) {
      this.selectionStart = start;
      this.selectionEnd = end;
    },
    focus() {},
    dispatchEvent() {
      return true;
    },
  };
  return editor as unknown as HTMLTextAreaElement;
}

test("declarative wrap actions own text formatting", () => {
  const editor = textarea("selected", 0, 8);

  applyEditorInsertAction(editor, {
    markdown: "~~",
    suffix: "~~",
    placeholder: "strikethrough text",
    mode: "wrap",
  });

  assert.equal(editor.value, "~~selected~~");
  assert.equal(editor.selectionStart, 2);
  assert.equal(editor.selectionEnd, 10);
});

test("declarative prefix actions own block formatting", () => {
  const editor = textarea("first\nsecond", 0, 12);

  applyEditorInsertAction(editor, {
    markdown: "- [ ] ",
    placeholder: "task",
    mode: "prefix-lines",
  });

  assert.equal(editor.value, "- [ ] first\n- [ ] second");
});
