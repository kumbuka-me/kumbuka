import { requiredElement, requiredElements } from "../../core/dom.ts";
// Markdown table creation, selection, and style controls.

const tableTones = new Set([
  "accent",
  "accent-soft",
  "info",
  "success",
  "warning",
  "danger",
  "neutral",
  "gray",
  "blue",
  "purple",
  "green",
  "yellow",
  "orange",
  "red",
]);

export type TableDirective = {
  widths?: number[];
  heights?: number[];
  header: string;
  rows: Record<string, string>;
  columns: Record<string, string>;
  cells: Record<string, string>;
  sortable: boolean;
  filterable: boolean;
};

type TableContextKind = "table" | "header" | "separator" | "body" | "directive";
type TableContext = { kind: TableContextKind; row: number; column: number };
export type MarkdownTable = {
  headerLine: number;
  separatorLine: number;
  endLine: number;
  directiveLine: number;
  directive: TableDirective;
  context: TableContext;
};

function emptyDirective(): TableDirective {
  return {
    header: "",
    rows: {},
    columns: {},
    cells: {},
    sortable: false,
    filterable: false,
  };
}

function cloneDirective(directive: TableDirective): TableDirective {
  return {
    ...(directive.widths ? { widths: [...directive.widths] } : {}),
    ...(directive.heights ? { heights: [...directive.heights] } : {}),
    header: directive.header || "",
    rows: { ...(directive.rows || {}) },
    columns: { ...(directive.columns || {}) },
    cells: { ...(directive.cells || {}) },
    sortable: Boolean(directive.sortable),
    filterable: Boolean(directive.filterable),
  };
}

function lineStarts(source: string): number[] {
  const starts = [0];

  for (let index = 0; index < source.length; index += 1) {
    if (source[index] === "\n") starts.push(index + 1);
  }

  return starts;
}

function lineAtOffset(starts: number[], offset: number): number {
  let low = 0;
  let high = starts.length - 1;

  while (low <= high) {
    const middle = Math.floor((low + high) / 2);
    if ((starts[middle] ?? 0) <= offset) low = middle + 1;
    else high = middle - 1;
  }

  return Math.max(0, high);
}

function cellParts(line: string): string[] {
  const trimmed = line.trim();
  if (!trimmed.includes("|")) return [];

  const parts: string[] = [];
  let current = "";
  let escaped = false;

  for (const char of trimmed) {
    if (escaped) {
      current += char;
      escaped = false;
      continue;
    }
    if (char === "\\") {
      current += char;
      escaped = true;
      continue;
    }
    if (char === "|") {
      parts.push(current);
      current = "";
      continue;
    }

    current += char;
  }

  parts.push(current);

  if (parts[0]?.trim() === "") parts.shift();
  if (parts.at(-1)?.trim() === "") parts.pop();

  return parts;
}

function isSeparatorCell(value: string): boolean {
  value = value.trim();

  if (value.startsWith(":")) value = value.slice(1);
  if (value.endsWith(":")) value = value.slice(0, -1);

  if (value.length < 3) return false;

  for (const char of value) if (char !== "-") return false;

  return true;
}

function isTableSeparator(line: string): boolean {
  const parts = cellParts(line);
  return parts.length > 0 && parts.every(isSeparatorCell);
}

function isTableRow(line: string): boolean {
  return line.trim() !== "" && cellParts(line).length > 0;
}

function cellIndexAtColumn(line: string, column: number): number {
  const leadingPipe = line.trimStart().startsWith("|");
  let pipes = 0;
  let escaped = false;

  for (let index = 0; index < Math.min(column, line.length); index += 1) {
    const char = line[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (char === "\\") {
      escaped = true;
      continue;
    }

    if (char === "|") pipes += 1;
  }

  return Math.max(1, leadingPipe ? pipes : pipes + 1);
}

function tableDirectiveLine(line: string): boolean {
  const trimmed = line.trim();
  return trimmed.startsWith("{table ") && trimmed.endsWith("}");
}

function validCellCoordinates(
  row: number,
  column: number,
  extra: string | undefined,
): boolean {
  if (extra !== undefined) return false;
  if (!Number.isInteger(row) || !Number.isInteger(column)) return false;

  return row >= 1 && column >= 1;
}

// Parses a table-format directive into structured state.
export function parseTableDirective(line: string): TableDirective | null {
  const trimmed = line.trim();
  if (!tableDirectiveLine(trimmed)) return null;

  const directive = emptyDirective();
  const body = trimmed.slice("{table".length, -1).trim();
  if (!body) return null;

  for (const token of body.split(/\s+/)) {
    if (token === "sortable") {
      directive.sortable = true;
      continue;
    }
    if (token === "filterable") {
      directive.filterable = true;
      continue;
    }

    const equals = token.indexOf("=");
    if (equals <= 0) return null;

    const key = token.slice(0, equals);
    const tone = token.slice(equals + 1);
    if (key === "widths" || key === "heights") {
      if (!/^\d+(,\d+)*$/.test(tone)) return null;
      const values = tone.split(",").map(Number);
      if (values.some((value) => value > 4000)) return null;
      directive[key] = values;
      continue;
    }
    if (!tableTones.has(tone)) return null;

    if (key === "header") {
      directive.header = tone;
      continue;
    }
    if (key.startsWith("row:")) {
      const row = Number.parseInt(key.slice(4), 10);
      if (!Number.isInteger(row) || row < 1) return null;

      directive.rows[String(row)] = tone;
      continue;
    }
    if (key.startsWith("col:") || key.startsWith("column:")) {
      const prefixLength = key.startsWith("column:") ? 7 : 4;
      const column = Number.parseInt(key.slice(prefixLength), 10);
      if (!Number.isInteger(column) || column < 1) return null;

      directive.columns[String(column)] = tone;
      continue;
    }
    if (key.startsWith("cell:")) {
      const [rowValue = "", columnValue = "", extra] = key.slice(5).split(",");
      const row = Number.parseInt(rowValue, 10);
      const column = Number.parseInt(columnValue, 10);
      if (!validCellCoordinates(row, column, extra)) return null;

      directive.cells[`${row},${column}`] = tone;
      continue;
    }

    return null;
  }

  return directive;
}

function numericEntries(values: Record<string, string>): [string, string][] {
  return Object.entries(values).sort(
    ([left], [right]) => Number(left) - Number(right),
  );
}

function cellEntries(values: Record<string, string>): [string, string][] {
  return Object.entries(values).sort(([left], [right]) => {
    const [leftRow = 0, leftColumn = 0] = left.split(",").map(Number);
    const [rightRow = 0, rightColumn = 0] = right.split(",").map(Number);

    return leftRow - rightRow || leftColumn - rightColumn;
  });
}

// Serializes table-format state back to Markdown.
export function serializeTableDirective(directive: TableDirective): string {
  const tokens: string[] = [];

  for (const key of ["widths", "heights"] as const)
    if (directive[key]?.length)
      tokens.push(`${key}=${directive[key]!.join(",")}`);
  if (directive.header) tokens.push(`header=${directive.header}`);
  for (const [column, tone] of numericEntries(directive.columns || {}))
    tokens.push(`col:${column}=${tone}`);
  for (const [row, tone] of numericEntries(directive.rows || {}))
    tokens.push(`row:${row}=${tone}`);
  for (const [cell, tone] of cellEntries(directive.cells || {}))
    tokens.push(`cell:${cell}=${tone}`);
  if (directive.sortable) tokens.push("sortable");
  if (directive.filterable) tokens.push("filterable");

  return tokens.length ? `{table ${tokens.join(" ")}}` : "";
}

function previousTableRow(lines: string[], start: number): number {
  let index = start;

  while (index >= 0 && (lines[index] ?? "").trim() === "") index -= 1;

  return index >= 0 && isTableRow(lines[index] ?? "") ? index : -1;
}

// Finds the Markdown table around the current selection.
export function findMarkdownTable(
  source: string,
  cursorOffset: number,
): MarkdownTable | null {
  const lines = source.split("\n");
  const starts = lineStarts(source);
  const safeOffset = Math.max(0, Math.min(cursorOffset, source.length));
  const cursorLine = lineAtOffset(starts, safeOffset);
  let probeLine = cursorLine;

  if (tableDirectiveLine(lines[probeLine] || "")) {
    probeLine = previousTableRow(lines, probeLine - 1);
    if (probeLine < 0) return null;
  }

  let separatorLine = -1;

  for (let index = probeLine; index >= 1; index -= 1) {
    if (
      isTableSeparator(lines[index] ?? "") &&
      isTableRow(lines[index - 1] ?? "")
    ) {
      separatorLine = index;
      break;
    }
    if (index < probeLine && (lines[index] ?? "").trim() === "") break;
  }
  if (separatorLine < 0) {
    for (
      let index = probeLine + 1;
      index < Math.min(lines.length, probeLine + 3);
      index += 1
    ) {
      if (
        isTableSeparator(lines[index] ?? "") &&
        isTableRow(lines[index - 1] ?? "")
      ) {
        separatorLine = index;
        break;
      }
    }
  }

  if (separatorLine < 1) return null;

  const headerLine = separatorLine - 1;
  let endLine = separatorLine;

  while (endLine + 1 < lines.length && isTableRow(lines[endLine + 1] ?? ""))
    endLine += 1;

  let directiveLine = endLine + 1;

  while (
    directiveLine < lines.length &&
    (lines[directiveLine] ?? "").trim() === ""
  )
    directiveLine += 1;

  const parsedDirective =
    directiveLine < lines.length
      ? parseTableDirective(lines[directiveLine] ?? "")
      : null;

  if (!parsedDirective) directiveLine = -1;

  const cursorInTable = cursorLine >= headerLine && cursorLine <= endLine;
  const cursorOnDirective = cursorLine === directiveLine;
  const cursorBetween =
    directiveLine >= 0 && cursorLine > endLine && cursorLine < directiveLine;
  if (!cursorInTable && !cursorOnDirective && !cursorBetween) return null;

  let kind: TableContextKind = "table";
  let row = 0;
  let column = 1;

  if (cursorLine === headerLine) kind = "header";
  else if (cursorLine === separatorLine) kind = "separator";
  else if (cursorLine > separatorLine && cursorLine <= endLine) {
    kind = "body";
    row = cursorLine - separatorLine;
  } else if (cursorOnDirective || cursorBetween) kind = "directive";

  if (cursorInTable && cursorLine !== separatorLine) {
    const currentLine = lines[cursorLine] ?? "";

    column = cellIndexAtColumn(
      currentLine,
      safeOffset - (starts[cursorLine] ?? 0),
    );
    column = Math.min(column, Math.max(1, cellParts(currentLine).length));
  }

  return {
    headerLine,
    separatorLine,
    endLine,
    directiveLine,
    directive: parsedDirective || emptyDirective(),
    context: { kind, row, column },
  };
}

// Enumerates tables in source order for visual table styles and selection mapping.
export function markdownTables(source: string): MarkdownTable[] {
  const tables: MarkdownTable[] = [];
  const lines = source.split("\n");
  const starts = lineStarts(source);
  let fenced = false;
  let fence = "";
  for (let index = 0; index < lines.length; index += 1) {
    const marker = /^\s*(`{3,}|~{3,})/.exec(lines[index] || "")?.[1];
    if (marker) {
      if (!fenced) {
        fenced = true;
        fence = marker;
      } else if (marker[0] === fence[0] && marker.length >= fence.length)
        fenced = false;
      continue;
    }
    if (fenced || !isTableSeparator(lines[index] || "")) continue;
    const table = findMarkdownTable(source, starts[index] || 0);
    if (table) {
      tables.push(table);
      index = table.endLine;
    }
  }
  return tables;
}

// Replaces or inserts the directive for one Markdown table.
export function rewriteTableDirectiveSource(
  source: string,
  table: MarkdownTable,
  directive: TableDirective,
): string {
  const lines = source.split("\n");
  const serialized = serializeTableDirective(directive);
  if (table.directiveLine >= 0) {
    if (serialized) lines[table.directiveLine] = serialized;
    else {
      lines.splice(table.directiveLine, 1);

      const afterTable = table.endLine + 1;

      if (
        afterTable < lines.length &&
        (lines[afterTable] ?? "").trim() === "" &&
        (lines[afterTable + 1] ?? "").trim() === ""
      )
        lines.splice(afterTable, 1);
    }
    return lines.join("\n");
  }
  if (!serialized) return source;

  lines.splice(table.endLine + 1, 0, "", serialized);
  return lines.join("\n");
}

function directiveTone(directive: TableDirective, target: string): string {
  if (target === "header") return directive.header || "";
  if (target.startsWith("row:")) return directive.rows[target.slice(4)] || "";
  if (target.startsWith("col:"))
    return directive.columns[target.slice(4)] || "";
  if (target.startsWith("cell:")) return directive.cells[target.slice(5)] || "";

  return "";
}

function setDirectiveTone(
  directive: TableDirective,
  target: string,
  tone: string,
): TableDirective {
  const next = cloneDirective(directive);

  if (target === "header") next.header = tone;
  else if (target.startsWith("row:")) {
    const key = target.slice(4);
    if (tone) next.rows[key] = tone;
    else delete next.rows[key];
  } else if (target.startsWith("col:")) {
    const key = target.slice(4);
    if (tone) next.columns[key] = tone;
    else delete next.columns[key];
  } else if (target.startsWith("cell:")) {
    const key = target.slice(5);
    if (tone) next.cells[key] = tone;
    else delete next.cells[key];
  }

  return next;
}

export function buildMarkdownTable(bodyRows: number, columns: number): string {
  const header = Array.from(
    { length: columns },
    (_, index) => `Column ${index + 1}`,
  );
  const separator = Array.from({ length: columns }, () => "---");
  const empty = Array.from({ length: columns }, () => "");
  const lines = [`| ${header.join(" | ")} |`, `| ${separator.join(" | ")} |`];

  for (let row = 0; row < bodyRows; row += 1)
    lines.push(`| ${empty.join(" | ")} |`);

  return lines.join("\n");
}

function replaceTextarea(
  textarea: HTMLTextAreaElement,
  nextValue: string,
  cursor: number,
): void {
  textarea.setRangeText(nextValue, 0, textarea.value.length, "preserve");

  const safeCursor = Math.max(0, Math.min(cursor, textarea.value.length));

  textarea.setSelectionRange(safeCursor, safeCursor);
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

function insertTable(
  textarea: HTMLTextAreaElement,
  rows: number,
  columns: number,
): void {
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;
  const before = textarea.value.slice(0, start);
  const after = textarea.value.slice(end);
  const prefix = before && !before.endsWith("\n") ? "\n" : "";
  const suffix = after && !after.startsWith("\n") ? "\n" : "";
  const markdown = buildMarkdownTable(rows, columns);

  textarea.setRangeText(`${prefix}${markdown}${suffix}`, start, end, "end");

  const firstCellStart = start + prefix.length + 2;

  textarea.setSelectionRange(
    firstCellStart,
    firstCellStart + "Column 1".length,
  );
  textarea.focus();
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

type TableEditResult = { source: string; cursor: number };

function formatTableRow(cells: string[]): string {
  return `| ${cells.map((cell) => cell.trim()).join(" | ")} |`;
}

function sourceLineOffset(lines: string[], line: number): number {
  let offset = 0;

  for (let index = 0; index < line; index += 1)
    offset += (lines[index]?.length ?? 0) + 1;

  return offset;
}

function tableCellOffset(line: string, column: number): number {
  const cells = cellParts(line).map((cell) => cell.trim());
  const safeColumn = Math.max(1, Math.min(column, Math.max(1, cells.length)));
  let offset = 2;

  for (let index = 0; index < safeColumn - 1; index += 1)
    offset += (cells[index]?.length ?? 0) + 3;

  return offset;
}

function tableColumnCount(lines: string[], table: MarkdownTable): number {
  return Math.max(1, cellParts(lines[table.headerLine] ?? "").length);
}

function removeDirectiveLine(lines: string[], table: MarkdownTable): void {
  if (table.directiveLine >= 0) lines.splice(table.directiveLine, 1);
}

function insertDirectiveAfterTable(
  lines: string[],
  endLine: number,
  directive: TableDirective,
): void {
  const serialized = serializeTableDirective(directive);
  if (!serialized) return;

  let insertAt = endLine + 1;
  if ((lines[insertAt] ?? "").trim() === "") insertAt += 1;
  else {
    lines.splice(insertAt, 0, "");
    insertAt += 1;
  }

  lines.splice(insertAt, 0, serialized);
}

function insertIndexedValue(
  values: Record<string, string>,
  at: number,
): Record<string, string> {
  const next: Record<string, string> = {};

  for (const [key, value] of numericEntries(values)) {
    const index = Number(key);
    next[String(index >= at ? index + 1 : index)] = value;
  }

  return next;
}

function deleteIndexedValue(
  values: Record<string, string>,
  at: number,
): Record<string, string> {
  const next: Record<string, string> = {};

  for (const [key, value] of numericEntries(values)) {
    const index = Number(key);
    if (index === at) continue;

    next[String(index > at ? index - 1 : index)] = value;
  }

  return next;
}

function remapCells(
  values: Record<string, string>,
  mapRow: (row: number) => number | null,
  mapColumn: (column: number) => number | null,
): Record<string, string> {
  const next: Record<string, string> = {};

  for (const [cell, tone] of cellEntries(values)) {
    const [row = 0, column = 0] = cell.split(",").map(Number);
    const nextRow = mapRow(row);
    const nextColumn = mapColumn(column);
    if (nextRow === null || nextColumn === null) continue;

    next[`${nextRow},${nextColumn}`] = tone;
  }

  return next;
}

function cursorForCell(lines: string[], line: number, column: number): number {
  if (line < 0 || line >= lines.length) return 0;

  return (
    sourceLineOffset(lines, line) + tableCellOffset(lines[line] ?? "", column)
  );
}

// Inserts an empty body row and shifts row/cell formatting with it.
export function insertMarkdownTableRow(
  source: string,
  table: MarkdownTable,
  row: number,
  after: boolean,
): TableEditResult {
  const lines = source.split("\n");
  const bodyRows = Math.max(0, table.endLine - table.separatorLine);
  const columns = tableColumnCount(lines, table);
  const anchor = bodyRows > 0 ? Math.max(1, Math.min(row, bodyRows)) : 1;
  const insertRow = bodyRows === 0 ? 1 : anchor + (after ? 1 : 0);
  const insertLine = table.separatorLine + insertRow;
  const directive = cloneDirective(table.directive);

  removeDirectiveLine(lines, table);
  lines.splice(insertLine, 0, formatTableRow(Array(columns).fill("")));

  directive.heights?.splice(insertRow, 0, 0);
  directive.rows = insertIndexedValue(directive.rows, insertRow);
  directive.cells = remapCells(
    directive.cells,
    (value) => (value >= insertRow ? value + 1 : value),
    (value) => value,
  );

  const endLine = table.endLine + 1;
  insertDirectiveAfterTable(lines, endLine, directive);

  return {
    source: lines.join("\n"),
    cursor: cursorForCell(lines, insertLine, 1),
  };
}

// Deletes one body row and compacts row/cell formatting coordinates.
export function deleteMarkdownTableRow(
  source: string,
  table: MarkdownTable,
  row: number,
): TableEditResult {
  const lines = source.split("\n");
  const bodyRows = Math.max(0, table.endLine - table.separatorLine);
  if (row < 1 || row > bodyRows)
    return { source, cursor: cursorForCell(lines, table.headerLine, 1) };

  const columns = tableColumnCount(lines, table);
  const deleteLine = table.separatorLine + row;
  const directive = cloneDirective(table.directive);

  removeDirectiveLine(lines, table);
  lines.splice(deleteLine, 1);

  directive.heights?.splice(row, 1);
  directive.rows = deleteIndexedValue(directive.rows, row);
  directive.cells = remapCells(
    directive.cells,
    (value) => (value === row ? null : value > row ? value - 1 : value),
    (value) => value,
  );

  const endLine = table.endLine - 1;
  insertDirectiveAfterTable(lines, endLine, directive);

  const remainingRows = bodyRows - 1;
  const targetLine =
    remainingRows > 0
      ? table.separatorLine + Math.min(row, remainingRows)
      : table.headerLine;

  return {
    source: lines.join("\n"),
    cursor: cursorForCell(
      lines,
      targetLine,
      Math.min(table.context.column, columns),
    ),
  };
}

// Inserts a column and shifts column/cell formatting with it.
export function insertMarkdownTableColumn(
  source: string,
  table: MarkdownTable,
  column: number,
  after: boolean,
): TableEditResult {
  const lines = source.split("\n");
  const columns = tableColumnCount(lines, table);
  const anchor = Math.max(1, Math.min(column, columns));
  const insertColumn = anchor + (after ? 1 : 0);
  const directive = cloneDirective(table.directive);

  removeDirectiveLine(lines, table);

  for (let line = table.headerLine; line <= table.endLine; line += 1) {
    const cells = cellParts(lines[line] ?? "");
    while (cells.length < columns) cells.push("");

    cells.splice(
      insertColumn - 1,
      0,
      line === table.separatorLine ? "---" : "",
    );
    lines[line] = formatTableRow(cells);
  }

  directive.widths?.splice(insertColumn - 1, 0, 0);
  directive.columns = insertIndexedValue(directive.columns, insertColumn);
  directive.cells = remapCells(
    directive.cells,
    (value) => value,
    (value) => (value >= insertColumn ? value + 1 : value),
  );
  insertDirectiveAfterTable(lines, table.endLine, directive);

  const targetLine =
    table.context.kind === "body"
      ? table.separatorLine + table.context.row
      : table.headerLine;

  return {
    source: lines.join("\n"),
    cursor: cursorForCell(lines, targetLine, insertColumn),
  };
}

// Deletes a column and compacts column/cell formatting coordinates.
export function deleteMarkdownTableColumn(
  source: string,
  table: MarkdownTable,
  column: number,
): TableEditResult {
  const lines = source.split("\n");
  const columns = tableColumnCount(lines, table);
  if (columns <= 1)
    return { source, cursor: cursorForCell(lines, table.headerLine, 1) };

  const deleteColumn = Math.max(1, Math.min(column, columns));
  const directive = cloneDirective(table.directive);

  removeDirectiveLine(lines, table);

  for (let line = table.headerLine; line <= table.endLine; line += 1) {
    const cells = cellParts(lines[line] ?? "");
    while (cells.length < columns) cells.push("");

    cells.splice(deleteColumn - 1, 1);
    lines[line] = formatTableRow(cells);
  }

  directive.widths?.splice(deleteColumn - 1, 1);
  directive.columns = deleteIndexedValue(directive.columns, deleteColumn);
  directive.cells = remapCells(
    directive.cells,
    (value) => value,
    (value) =>
      value === deleteColumn ? null : value > deleteColumn ? value - 1 : value,
  );
  insertDirectiveAfterTable(lines, table.endLine, directive);

  const targetLine =
    table.context.kind === "body"
      ? table.separatorLine + table.context.row
      : table.headerLine;
  const targetColumn = Math.min(deleteColumn, columns - 1);

  return {
    source: lines.join("\n"),
    cursor: cursorForCell(lines, targetLine, targetColumn),
  };
}

// Clears the current header/body cell without changing table structure.
export function clearMarkdownTableCell(
  source: string,
  table: MarkdownTable,
): TableEditResult {
  const lines = source.split("\n");
  const { kind, row, column } = table.context;
  const targetLine =
    kind === "header"
      ? table.headerLine
      : kind === "body"
        ? table.separatorLine + row
        : -1;
  if (targetLine < 0) return { source, cursor: 0 };

  const cells = cellParts(lines[targetLine] ?? "");
  if (column < 1 || column > cells.length)
    return { source, cursor: cursorForCell(lines, targetLine, 1) };

  cells[column - 1] = "";
  lines[targetLine] = formatTableRow(cells);

  return {
    source: lines.join("\n"),
    cursor: cursorForCell(lines, targetLine, column),
  };
}

// Removes the complete table and its optional formatting directive.
export function deleteMarkdownTable(
  source: string,
  table: MarkdownTable,
): TableEditResult {
  const lines = source.split("\n");
  const endLine =
    table.directiveLine >= 0 ? table.directiveLine : table.endLine;

  lines.splice(table.headerLine, endLine - table.headerLine + 1);

  if (
    table.headerLine > 0 &&
    table.headerLine < lines.length &&
    (lines[table.headerLine - 1] ?? "").trim() === "" &&
    (lines[table.headerLine] ?? "").trim() === ""
  )
    lines.splice(table.headerLine, 1);

  const cursor =
    table.headerLine < lines.length
      ? sourceLineOffset(lines, table.headerLine)
      : lines.join("\n").length;

  return { source: lines.join("\n"), cursor };
}

function directiveTarget(table: MarkdownTable, target: string): string | null {
  const { kind, row, column } = table.context;

  if (target === "header") return "header";
  if (target === "row" && kind === "body") return `row:${row}`;
  if (target === "column" && (kind === "header" || kind === "body"))
    return `col:${column}`;
  if (target === "cell" && kind === "body") return `cell:${row},${column}`;

  return null;
}

function setupTablePalette(toolbar: HTMLElement): void {
  const editor = requiredElement<HTMLTextAreaElement>(
    document,
    "[data-markdown-editor]",
  );
  const open = requiredElement<HTMLButtonElement>(
    toolbar,
    "[data-table-format-open]",
  );
  const insertOwner = requiredElement<HTMLElement>(
    toolbar,
    "[data-table-insert-owner]",
  );
  const insertPopover = requiredElement<HTMLElement>(
    insertOwner,
    "[data-table-insert-popover]",
  );
  const insertGrid = requiredElement<HTMLElement>(
    insertPopover,
    "[data-table-insert-grid]",
  );
  const insertSize = requiredElement<HTMLElement>(
    insertPopover,
    "[data-table-insert-size]",
  );
  const contextToolbar = requiredElement<HTMLElement>(
    document,
    "[data-table-context-toolbar]",
  );
  const contextLabel = requiredElement<HTMLElement>(
    contextToolbar,
    "[data-table-context-label]",
  );
  const actionButtons = requiredElements<HTMLButtonElement>(
    contextToolbar,
    "[data-table-action]",
  );
  const colorTarget = requiredElement<HTMLSelectElement>(
    contextToolbar,
    "[data-table-color-target]",
  );
  const toneButtons = requiredElements<HTMLButtonElement>(
    contextToolbar,
    "[data-table-tone]",
  );
  const sortableControl = requiredElement<HTMLInputElement>(
    contextToolbar,
    "[data-table-format-sortable]",
  );
  const filterableControl = requiredElement<HTMLInputElement>(
    contextToolbar,
    "[data-table-format-filterable]",
  );
  const moreMenu = requiredElement<HTMLDetailsElement>(
    contextToolbar,
    "[data-table-context-more]",
  );
  let currentTable: MarkdownTable | null = null;
  let selectedRows = 3;
  let selectedColumns = 3;

  function syncInsertGrid(rows: number, columns: number): void {
    selectedRows = rows;
    selectedColumns = columns;
    insertSize.textContent = `${rows} × ${columns}`;

    for (const cell of insertGrid.querySelectorAll<HTMLButtonElement>(
      "[data-table-size-cell]",
    )) {
      const cellRows = Number.parseInt(cell.dataset.rows || "0", 10);
      const cellColumns = Number.parseInt(cell.dataset.columns || "0", 10);
      const active = cellRows <= rows && cellColumns <= columns;

      cell.classList.toggle("active", active);
      cell.setAttribute(
        "aria-pressed",
        String(cellRows === rows && cellColumns === columns),
      );
    }
  }

  function buildInsertGrid(): void {
    const fragment = document.createDocumentFragment();

    for (let rows = 1; rows <= 10; rows += 1) {
      for (let columns = 1; columns <= 10; columns += 1) {
        const cell = document.createElement("button");
        cell.type = "button";
        cell.className = "table-size-cell";
        cell.dataset.tableSizeCell = "true";
        cell.dataset.rows = String(rows);
        cell.dataset.columns = String(columns);
        cell.setAttribute("role", "gridcell");
        cell.setAttribute(
          "aria-label",
          `${rows} body rows by ${columns} columns`,
        );
        cell.addEventListener("mouseenter", () =>
          syncInsertGrid(rows, columns),
        );
        cell.addEventListener("focus", () => syncInsertGrid(rows, columns));
        cell.addEventListener("click", () => {
          closeInsertPopover();
          const form = editor.closest<HTMLFormElement>("[data-editor-form]");
          if (form?.dataset.editorMode === "visual") {
            form.dispatchEvent(
              new CustomEvent("editor:visual-table-insert", {
                detail: { rows, columns },
              }),
            );
            return;
          }
          insertTable(editor, rows, columns);
          refreshContext();
        });
        fragment.append(cell);
      }
    }

    insertGrid.replaceChildren(fragment);
    syncInsertGrid(selectedRows, selectedColumns);
  }

  function closeInsertPopover(): void {
    insertPopover.hidden = true;
    open.setAttribute("aria-expanded", "false");
  }

  function openInsertPopover(): void {
    syncInsertGrid(selectedRows, selectedColumns);
    insertPopover.hidden = false;
    open.setAttribute("aria-expanded", "true");
  }

  function selectedDirectiveTarget(): string | null {
    return currentTable
      ? directiveTarget(currentTable, colorTarget.value)
      : null;
  }

  function syncTone(): void {
    const target = selectedDirectiveTarget();
    const tone =
      currentTable && target
        ? directiveTone(currentTable.directive, target)
        : "";

    for (const button of toneButtons) {
      const active = (button.dataset.tableTone ?? "") === tone;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    }
  }

  function syncColorTarget(): void {
    if (!currentTable) return;

    const { kind } = currentTable.context;
    const enabled = new Set<string>(["header"]);
    if (kind === "header" || kind === "body") enabled.add("column");
    if (kind === "body") {
      enabled.add("cell");
      enabled.add("row");
    }

    for (const option of colorTarget.options)
      option.disabled = !enabled.has(option.value);

    if (!enabled.has(colorTarget.value))
      colorTarget.value = kind === "body" ? "cell" : "header";

    syncTone();
  }

  let visualContext: {
    tableIndex: number;
    kind: "header" | "body";
    row: number;
    column: number;
  } | null = null;
  const form = editor.closest<HTMLFormElement>("[data-editor-form]");
  form?.addEventListener("editor:visual-table-context", (event) => {
    const next = (event as CustomEvent).detail;
    if (next && next.kind !== visualContext?.kind)
      colorTarget.value = next.kind === "body" ? "cell" : "header";
    visualContext = next;
    refreshContext();
  });
  form?.addEventListener("editor:mode-change", () => refreshContext());

  function refreshContext(): void {
    if (form?.dataset.editorMode === "visual") {
      currentTable = visualContext
        ? markdownTables(editor.value)[visualContext.tableIndex] || null
        : null;
      if (currentTable && visualContext)
        currentTable.context = { ...visualContext };
    } else
      currentTable = findMarkdownTable(
        editor.value,
        editor.selectionStart ?? 0,
      );
    const kind = currentTable?.context.kind;
    const visible =
      currentTable !== null &&
      (kind === "header" || kind === "separator" || kind === "body");

    contextToolbar.hidden = !visible;
    if (!visible || !currentTable) return;

    const { row, column } = currentTable.context;
    if (kind === "header")
      contextLabel.textContent = `Table · Header · Column ${column}`;
    else if (kind === "body")
      contextLabel.textContent = `Table · Row ${row} · Column ${column}`;
    else contextLabel.textContent = "Table · Separator";

    const lines = editor.value.split("\n");
    const columns = tableColumnCount(lines, currentTable);
    const onBody = kind === "body";
    const onCell = kind === "body" || kind === "header";

    for (const button of actionButtons) {
      const action = button.dataset.tableAction;
      if (action === "insert-row-above" || action === "insert-row-below")
        button.disabled = !onBody;
      else if (action === "delete-row") button.disabled = !onBody;
      else if (
        action === "insert-column-left" ||
        action === "insert-column-right"
      )
        button.disabled = !onCell;
      else if (action === "delete-column")
        button.disabled = !onCell || columns <= 1;
      else if (action === "clear-cell") button.disabled = !onCell;
    }

    sortableControl.checked = currentTable.directive.sortable;
    filterableControl.checked = currentTable.directive.filterable;
    syncColorTarget();
  }

  function writeDirective(nextDirective: TableDirective): void {
    if (!currentTable) return;

    const cursor = editor.selectionStart ?? 0;
    const nextValue = rewriteTableDirectiveSource(
      editor.value,
      currentTable,
      nextDirective,
    );

    replaceTextarea(editor, nextValue, cursor);
    editor.focus();
    refreshContext();
  }

  function applyTableEdit(result: TableEditResult): void {
    replaceTextarea(editor, result.source, result.cursor);
    editor.focus();
    refreshContext();
  }

  open.addEventListener("click", () => {
    refreshContext();
    if (!contextToolbar.hidden) {
      closeInsertPopover();
      return;
    }

    if (insertPopover.hidden) openInsertPopover();
    else closeInsertPopover();
  });

  for (const eventName of ["input", "keyup", "click", "select", "focus"])
    editor.addEventListener(eventName, refreshContext);

  colorTarget.addEventListener("change", syncTone);

  for (const button of toneButtons)
    button.addEventListener("click", () => {
      if (!currentTable || button.disabled) return;

      const target = selectedDirectiveTarget();
      if (!target) return;

      writeDirective(
        setDirectiveTone(
          currentTable.directive,
          target,
          button.dataset.tableTone ?? "",
        ),
      );
    });

  sortableControl.addEventListener("change", () => {
    if (!currentTable || sortableControl.disabled) return;

    const next = cloneDirective(currentTable.directive);
    next.sortable = sortableControl.checked;
    writeDirective(next);
  });

  filterableControl.addEventListener("change", () => {
    if (!currentTable || filterableControl.disabled) return;

    const next = cloneDirective(currentTable.directive);
    next.filterable = filterableControl.checked;
    writeDirective(next);
  });

  for (const button of actionButtons)
    button.addEventListener("click", () => {
      if (!currentTable || button.disabled) return;

      const { row, column } = currentTable.context;
      switch (button.dataset.tableAction) {
        case "insert-row-above":
          applyTableEdit(
            insertMarkdownTableRow(editor.value, currentTable, row, false),
          );
          break;
        case "insert-row-below":
          applyTableEdit(
            insertMarkdownTableRow(editor.value, currentTable, row, true),
          );
          break;
        case "delete-row":
          applyTableEdit(
            deleteMarkdownTableRow(editor.value, currentTable, row),
          );
          break;
        case "insert-column-left":
          applyTableEdit(
            insertMarkdownTableColumn(
              editor.value,
              currentTable,
              column,
              false,
            ),
          );
          break;
        case "insert-column-right":
          applyTableEdit(
            insertMarkdownTableColumn(editor.value, currentTable, column, true),
          );
          break;
        case "delete-column":
          applyTableEdit(
            deleteMarkdownTableColumn(editor.value, currentTable, column),
          );
          break;
        case "clear-cell":
          applyTableEdit(clearMarkdownTableCell(editor.value, currentTable));
          break;
        case "clear-colors": {
          const next = cloneDirective(currentTable.directive);
          next.header = "";
          next.rows = {};
          next.columns = {};
          next.cells = {};
          writeDirective(next);
          moreMenu.open = false;
          break;
        }
        case "delete-table":
          applyTableEdit(deleteMarkdownTable(editor.value, currentTable));
          moreMenu.open = false;
          break;
      }
    });

  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Node)) return;

    if (!insertOwner.contains(target)) closeInsertPopover();
    if (!moreMenu.contains(target)) moreMenu.open = false;
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;

    if (!insertPopover.hidden) {
      event.preventDefault();
      closeInsertPopover();
      open.focus();
    }
    moreMenu.open = false;
  });

  insertGrid.addEventListener("mouseleave", () =>
    syncInsertGrid(selectedRows, selectedColumns),
  );

  buildInsertGrid();
  refreshContext();
}

// Initializes table palette.
export function initTablePalette(): void {
  for (const toolbar of document.querySelectorAll<HTMLElement>(
    "[data-markdown-toolbar]",
  ))
    setupTablePalette(toolbar);
}
