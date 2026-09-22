import test from "node:test";
import assert from "node:assert/strict";

import {
  buildMarkdownTable,
  clearMarkdownTableCell,
  deleteMarkdownTable,
  deleteMarkdownTableColumn,
  deleteMarkdownTableRow,
  findMarkdownTable,
  markdownTables,
  insertMarkdownTableColumn,
  insertMarkdownTableRow,
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

test("table insertion builds the selected Confluence-style grid size", () => {
  assert.equal(
    buildMarkdownTable(2, 3),
    [
      "| Column 1 | Column 2 | Column 3 |",
      "| --- | --- | --- |",
      "|  |  |  |",
      "|  |  |  |",
    ].join("\n"),
  );
});

test("row insertion shifts row and cell color directives", () => {
  const source = [
    "| Service | Status |",
    "| --- | --- |",
    "| API | Healthy |",
    "| DB | Warning |",
    "",
    "{table row:2=yellow cell:2,2=red}",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Healthy"));

  assert.ok(table);

  const result = insertMarkdownTableRow(source, table, 1, true);

  assert.equal(
    result.source,
    [
      "| Service | Status |",
      "| --- | --- |",
      "| API | Healthy |",
      "|  |  |",
      "| DB | Warning |",
      "",
      "{table row:3=yellow cell:3,2=red}",
    ].join("\n"),
  );
});

test("column deletion compacts column and cell color directives", () => {
  const source = [
    "| Service | Status | Owner |",
    "| --- | --- | --- |",
    "| API | Healthy | Platform |",
    "",
    "{table col:2=yellow col:3=blue cell:1,2=red cell:1,3=green}",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Healthy"));

  assert.ok(table);

  const result = deleteMarkdownTableColumn(source, table, 2);

  assert.equal(
    result.source,
    [
      "| Service | Owner |",
      "| --- | --- |",
      "| API | Platform |",
      "",
      "{table col:2=blue cell:1,2=green}",
    ].join("\n"),
  );
});

test("clear cell keeps the table structure intact", () => {
  const source = [
    "| Service | Status |",
    "| --- | --- |",
    "| API | Healthy |",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Healthy"));

  assert.ok(table);

  const result = clearMarkdownTableCell(source, table);

  assert.equal(
    result.source,
    ["| Service | Status |", "| --- | --- |", "| API |  |"].join("\n"),
  );
});

test("delete table removes its formatting directive and preserves surrounding text", () => {
  const source = [
    "Before",
    "",
    "| Service | Status |",
    "| --- | --- |",
    "| API | Healthy |",
    "",
    "{table header=blue sortable}",
    "",
    "After",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Healthy"));

  assert.ok(table);

  const result = deleteMarkdownTable(source, table);

  assert.equal(result.source, ["Before", "", "After"].join("\n"));
});

test("row deletion compacts row and cell color directives", () => {
  const source = [
    "| Service | Status |",
    "| --- | --- |",
    "| API | Healthy |",
    "| DB | Warning |",
    "| Cache | Healthy |",
    "",
    "{table row:2=yellow row:3=green cell:2,2=red cell:3,2=blue}",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Warning"));

  assert.ok(table);

  const result = deleteMarkdownTableRow(source, table, 2);

  assert.equal(
    result.source,
    [
      "| Service | Status |",
      "| --- | --- |",
      "| API | Healthy |",
      "| Cache | Healthy |",
      "",
      "{table row:2=green cell:2,2=blue}",
    ].join("\n"),
  );
});

test("column insertion shifts column and cell color directives", () => {
  const source = [
    "| Service | Owner |",
    "| --- | --- |",
    "| API | Platform |",
    "",
    "{table col:2=blue cell:1,2=green}",
  ].join("\n");
  const table = findMarkdownTable(source, source.indexOf("Service"));

  assert.ok(table);

  const result = insertMarkdownTableColumn(source, table, 1, true);

  assert.equal(
    result.source,
    [
      "| Service |  | Owner |",
      "| --- | --- | --- |",
      "| API |  | Platform |",
      "",
      "{table col:3=blue cell:1,3=green}",
    ].join("\n"),
  );
});

test("visual table mapping skips fenced examples and retains directives in source order", () => {
  const source = [
    "```markdown",
    buildMarkdownTable(1, 2),
    "```",
    "",
    buildMarkdownTable(2, 3),
    "{table header=blue}",
    "",
    "~~~",
    buildMarkdownTable(1, 1),
    "~~~",
    "",
    buildMarkdownTable(1, 2),
    "{table header=red}",
  ].join("\n");
  const tables = markdownTables(source);
  assert.equal(tables.length, 2);
  assert.deepEqual(
    tables.map((table) => table.directive.header),
    ["blue", "red"],
  );
});

test("table dimensions round-trip and follow inserted rows and columns", () => {
  const source =
    buildMarkdownTable(2, 2) +
    "\n\n{table widths=120,180 heights=32,64,48 cell:1,1=blue}";
  const table = findMarkdownTable(source, 0)!;
  assert.deepEqual(table.directive.widths, [120, 180]);
  assert.equal(
    serializeTableDirective(table.directive),
    "{table widths=120,180 heights=32,64,48 cell:1,1=blue}",
  );
  const rows = insertMarkdownTableRow(source, table, 1, false);
  assert.deepEqual(
    findMarkdownTable(rows.source, 0)?.directive.heights,
    [32, 0, 64, 48],
  );
  const columns = insertMarkdownTableColumn(source, table, 1, false);
  assert.deepEqual(
    findMarkdownTable(columns.source, 0)?.directive.widths,
    [0, 120, 180],
  );
  assert.equal(parseTableDirective("{table widths=5000}"), null);
  assert.equal(parseTableDirective("{table heights=1px}"), null);
});
