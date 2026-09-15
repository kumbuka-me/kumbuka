import test from "node:test";
import assert from "node:assert/strict";

import {
  findMarkdownTable,
  parseTableDirective,
  rewriteTableDirectiveSource,
  serializeTableDirective,
} from "../../web/src/ts/features/editor/tables.ts";

test("table directives round-trip colors and interaction options", () => {
  const source =
    "{table header=blue col:1=gray row:2=yellow cell:2,2=red sortable filterable}";
  const directive = parseTableDirective(source);

  assert.ok(directive);
  assert.equal(directive.header, "blue");
  assert.equal(directive.columns[1], "gray");
  assert.equal(directive.rows[2], "yellow");
  assert.equal(directive.cells["2,2"], "red");
  assert.equal(directive.sortable, true);
  assert.equal(directive.filterable, true);
  assert.equal(serializeTableDirective(directive), source);
});

test("table context resolves body row, column and existing directive", () => {
  const source = [
    "Before",
    "",
    "| Service | Status | Owner |",
    "| --- | --- | --- |",
    "| API | Healthy | Platform |",
    "| Database | Warning | Data |",
    "",
    "{table header=blue cell:2,2=red sortable}",
    "",
    "After",
  ].join("\n");
  const cursor = source.indexOf("Warning") + 2;
  const table = findMarkdownTable(source, cursor);

  assert.ok(table);
  assert.deepEqual(table.context, { kind: "body", row: 2, column: 2 });
  assert.equal(table.directive.header, "blue");
  assert.equal(table.directive.cells["2,2"], "red");
  assert.equal(table.directive.sortable, true);
});

test("rewriting a table directive inserts and removes only the directive", () => {
  const source = `| A | B |\n| --- | --- |\n| 1 | 2 |\n\nNext`;
  const table = findMarkdownTable(source, source.indexOf("2"));

  assert.ok(table);

  const withDirective = rewriteTableDirectiveSource(source, table, {
    header: "blue",
    rows: {},
    columns: {},
    cells: {},
    sortable: true,
    filterable: false,
  });

  assert.equal(
    withDirective,
    `| A | B |\n| --- | --- |\n| 1 | 2 |\n\n{table header=blue sortable}\n\nNext`,
  );

  const updatedTable = findMarkdownTable(
    withDirective,
    withDirective.indexOf("2"),
  );

  assert.ok(updatedTable);

  const cleared = rewriteTableDirectiveSource(withDirective, updatedTable, {
    header: "",
    rows: {},
    columns: {},
    cells: {},
    sortable: false,
    filterable: false,
  });

  assert.equal(cleared, source);
});

test("palette parser preserves existing semantic table tones", () => {
  const source =
    "{table header=accent col:2=info row:1=warning cell:1,2=danger filterable}";
  const directive = parseTableDirective(source);

  assert.ok(directive);
  assert.equal(serializeTableDirective(directive), source);
});
