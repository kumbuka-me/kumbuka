import test from "node:test";
import assert from "node:assert/strict";
import {
  mermaidAlignment,
  withMermaidAlignment,
} from "../../web/src/ts/core/mermaid-alignment.ts";

test("Mermaid defaults to centered presentation", () => {
  const source = "flowchart LR\nA --> B";
  assert.equal(mermaidAlignment(source), "center");
  assert.equal(withMermaidAlignment(source, "center"), source);
});

test("Mermaid alignment is a round-trippable source comment", () => {
  const source = "flowchart LR\nA --> B";
  const left = withMermaidAlignment(source, "left");
  assert.equal(left, "%% kumbuka-align: left\n" + source);
  assert.equal(mermaidAlignment(left), "left");
  const right = withMermaidAlignment(left, "right");
  assert.equal(right, "%% kumbuka-align: right\n" + source);
  assert.equal(mermaidAlignment(right), "right");
  assert.equal(withMermaidAlignment(right, "center"), source);
});

test("Mermaid alignment ignores comments in the diagram body", () => {
  const source = "flowchart LR\n%% kumbuka-align: right\nA --> B";
  assert.equal(mermaidAlignment(source), "center");
  assert.equal(
    withMermaidAlignment(source, "left"),
    "%% kumbuka-align: left\n" + source,
  );
});

test("Mermaid alignment does not duplicate existing directives", () => {
  const source = "%% kumbuka-align: LEFT\r\nflowchart LR";
  assert.equal(mermaidAlignment(source), "left");
  assert.equal(
    withMermaidAlignment(source, "left"),
    "%% kumbuka-align: left\nflowchart LR",
  );
});
