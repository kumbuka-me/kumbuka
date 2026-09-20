import assert from "node:assert/strict";
import test from "node:test";
import { initEditorSearch } from "../../web/src/ts/features/editor/search.ts";

// Minimal controls let us exercise the actual click handlers without a browser.
class Control extends EventTarget {
  value = "";
  checked = false;
  hidden = false;
  textContent = "";
  selectionStart = 0;
  selectionEnd = 0;
  controls = new Map<string, Control>();
  querySelector(selector: string): Control | null {
    return this.controls.get(selector) || null;
  }
  focus(): void {}
  setSelectionRange(start: number, end: number): void {
    this.selectionStart = start;
    this.selectionEnd = end;
  }
  setRangeText(value: string, start: number, end: number): void {
    this.value = this.value.slice(0, start) + value + this.value.slice(end);
    this.setSelectionRange(start + value.length, start + value.length);
  }
}

function searchFixture(run: (controls: Record<string, Control>) => void): void {
  const names = [
    "find",
    "replace",
    "find-case",
    "find-status",
    "find-close",
    "find-next",
    "find-previous",
    "replace-one",
    "replace-all",
  ];
  const controls: Record<string, Control> = {};
  const panel = new Control();
  for (const name of names) {
    const control = new Control();
    controls[name] = control;
    panel.controls.set(`[data-editor-${name}]`, control);
  }
  const source = new Control();
  controls.source = source;
  const form = new Control();
  form.controls.set("[data-editor-search]", panel);
  form.controls.set("[data-markdown-editor]", source);
  const original = globalThis.document;
  Object.assign(globalThis, { document: { querySelectorAll: () => [form] } });
  try {
    initEditorSearch();
    run(controls);
  } finally {
    Object.assign(globalThis, { document: original });
  }
}

void test("Replace with an empty query leaves the document untouched", () => {
  searchFixture((controls) => {
    controls.source.value = "alpha";
    controls.source.setSelectionRange(5, 5);
    controls.replace.value = "X";
    let changes = 0;
    controls.source.addEventListener("input", () => {
      changes++;
    });
    controls["replace-one"].dispatchEvent(new Event("click"));
    assert.equal(controls.source.value, "alpha");
    assert.equal(changes, 0);
    assert.equal(controls["find-status"].textContent, "Enter text to find.");
  });
});

void test("Previous match wraps from the start of the document", () => {
  searchFixture((controls) => {
    controls.source.value = "alpha\nbeta\nalpha";
    controls.find.value = "alpha";
    controls.source.setSelectionRange(0, 5);
    controls["find-previous"].dispatchEvent(new Event("click"));
    assert.equal(controls.source.selectionStart, 11);
    assert.equal(controls.source.selectionEnd, 16);
    assert.equal(controls["find-status"].textContent, "Match on line 3.");
    controls["find-previous"].dispatchEvent(new Event("click"));
    assert.equal(controls.source.selectionStart, 0);
    assert.equal(controls.source.selectionEnd, 5);
  });
});
