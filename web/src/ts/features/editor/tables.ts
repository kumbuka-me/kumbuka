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

type TableTargetInput = HTMLInputElement & {
  dataset: DOMStringMap & { tableTarget?: string };
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

function buildMarkdownTable(bodyRows: number, columns: number): string {
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

function setupTablePalette(toolbar: HTMLElement): void {
  const editor = requiredElement<HTMLTextAreaElement>(
    document,
    "[data-markdown-editor]",
  );
  const tableDialog = requiredElement<HTMLDialogElement>(
    document,
    "[data-table-format-dialog]",
  );
  const open = requiredElement<HTMLButtonElement>(
    toolbar,
    "[data-table-format-open]",
  );
  const existing = requiredElement<HTMLElement>(
    tableDialog,
    "[data-table-format-existing]",
  );
  const insertControls = requiredElement<HTMLElement>(
    tableDialog,
    "[data-table-format-insert]",
  );
  const tableContext = requiredElement<HTMLElement>(
    tableDialog,
    "[data-table-format-context]",
  );
  const targetInputs = requiredElements<TableTargetInput>(
    tableDialog,
    '[name="table-format-target"]',
  );
  const toneInputs = requiredElements<HTMLInputElement>(
    tableDialog,
    '[name="table-format-tone"]',
  );
  const sortableControl = requiredElement<HTMLInputElement>(
    tableDialog,
    "[data-table-format-sortable]",
  );
  const filterableControl = requiredElement<HTMLInputElement>(
    tableDialog,
    "[data-table-format-filterable]",
  );
  const clearButton = requiredElement<HTMLButtonElement>(
    tableDialog,
    "[data-table-format-clear]",
  );
  const rowInput = requiredElement<HTMLInputElement>(
    tableDialog,
    "[data-table-insert-rows]",
  );
  const columnInput = requiredElement<HTMLInputElement>(
    tableDialog,
    "[data-table-insert-columns]",
  );
  const insertButton = requiredElement<HTMLButtonElement>(
    tableDialog,
    "[data-table-insert-submit]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    tableDialog,
    "[data-table-format-close]",
  );
  let currentTable: MarkdownTable | null = null;

  function selectedTarget(): string {
    return (
      targetInputs.find((input) => input.checked && !input.disabled)?.value ||
      "header"
    );
  }

  function syncTone(directive: TableDirective): void {
    const tone = directiveTone(directive, selectedTarget());
    for (const input of toneInputs) input.checked = input.value === tone;
  }

  function setTarget(
    input: TableTargetInput | undefined,
    value: string,
    label: string,
    enabled: boolean,
  ): void {
    if (!input) return;

    input.value = value;
    input.disabled = !enabled;

    const targetName = input.dataset.tableTarget;
    const text = targetName
      ? tableDialog.querySelector<HTMLElement>(
          `[data-table-target-label="${targetName}"]`,
        )
      : null;

    if (text) text.textContent = label;
  }

  function refresh(preferredTarget = ""): void {
    currentTable = findMarkdownTable(editor.value, editor.selectionStart ?? 0);
    existing.hidden = !currentTable;
    insertControls.hidden = Boolean(currentTable);
    if (!currentTable) return;

    const { kind, row, column } = currentTable.context;

    if (kind === "header")
      tableContext.textContent = `Header · column ${column}`;
    else if (kind === "body")
      tableContext.textContent = `Body row ${row} · column ${column}`;
    else tableContext.textContent = "Table options";

    const byKind = Object.fromEntries(
      targetInputs.map((input) => [input.dataset.tableTarget || "", input]),
    ) as Record<string, TableTargetInput>;

    setTarget(byKind.header, "header", "Header", true);
    setTarget(byKind.row, `row:${row}`, `Row ${row}`, kind === "body");
    setTarget(
      byKind.column,
      `col:${column}`,
      `Column ${column}`,
      kind !== "separator",
    );
    setTarget(
      byKind.cell,
      `cell:${row},${column}`,
      `Cell ${row},${column}`,
      kind === "body",
    );

    const desired =
      preferredTarget || (kind === "body" ? `cell:${row},${column}` : "header");
    let selected = targetInputs.find(
      (input) => !input.disabled && input.value === desired,
    );

    selected ||= targetInputs.find((input) => !input.disabled);

    if (selected) selected.checked = true;

    sortableControl.checked = currentTable.directive.sortable;
    filterableControl.checked = currentTable.directive.filterable;
    syncTone(currentTable.directive);
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
    refresh(selectedTarget());
  }

  open.addEventListener("click", () => {
    refresh();
    tableDialog.showModal();
  });

  for (const button of closeButtons)
    button.addEventListener("click", () => tableDialog.close());

  tableDialog.addEventListener("click", (event) => {
    if (event.target === tableDialog) tableDialog.close();
  });

  for (const input of targetInputs)
    input.addEventListener("change", () => {
      if (currentTable) syncTone(currentTable.directive);
    });
  for (const input of toneInputs)
    input.addEventListener("change", () => {
      if (!currentTable || input.disabled) return;
      writeDirective(
        setDirectiveTone(currentTable.directive, selectedTarget(), input.value),
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
  clearButton.addEventListener("click", () => {
    if (currentTable) writeDirective(emptyDirective());
  });
  insertButton.addEventListener("click", () => {
    const rowCount = Math.max(
      1,
      Math.min(20, Number.parseInt(rowInput.value, 10) || 3),
    );
    const columnCount = Math.max(
      1,
      Math.min(10, Number.parseInt(columnInput.value, 10) || 3),
    );

    rowInput.value = String(rowCount);
    columnInput.value = String(columnCount);
    tableDialog.close();
    insertTable(editor, rowCount, columnCount);
    refresh();
    tableDialog.showModal();
  });
}

// Initializes table palette.
export function initTablePalette(): void {
  for (const toolbar of document.querySelectorAll<HTMLElement>(
    "[data-markdown-toolbar]",
  ))
    setupTablePalette(toolbar);
}
