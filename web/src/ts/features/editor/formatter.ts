// Conservative Markdown document formatting.

interface Fence {
  character: string;
  length: number;
}

interface TableAlignment {
  left: boolean;
  right: boolean;
}

interface MarkdownLine {
  text: string;
  protected: boolean;
}

interface FormattedTable {
  lines: string[];
  end: number;
}

function openingFence(line: string): Fence | null {
  const match = line.match(/^ {0,3}(`{3,}|~{3,})(?:[^`~].*)?$/);
  if (!match?.[1]) return null;

  return { character: match[1][0] ?? "`", length: match[1].length };
}

// Reports whether a line closes the active Markdown fence.
function closesFence(line: string, fence: Fence): boolean {
  const trimmed = line.trim();
  if (!trimmed || trimmed[0] !== fence.character) return false;

  let length = 0;

  while (trimmed[length] === fence.character) length += 1;

  return length >= fence.length && trimmed.slice(length).trim() === "";
}

// Normalizes whitespace on a safe Markdown line.
function normalizeEditableLine(line: string): string {
  if (line.trim() === "") return "";

  // Four-space and tab-indented blocks can be code, tab bodies, or details
  // bodies. Leave their contents byte-for-byte intact rather than guessing.
  if (/^( {4}|\t)/.test(line)) return line;

  const hardBreak = / {2,}$/.test(line);
  let value = line.replace(/[ \t]+$/, "");

  value = value.replace(/^( {0,3}#{1,6})[ \t]+/, "$1 ");
  value = value.replace(/^( {0,3})[-+*][ \t]+/, "$1- ");
  value = value.replace(/^( {0,3}\d+[.)])[ \t]+/, "$1 ");
  value = value.replace(/^( {0,3}>)[ \t]*/, "$1 ");

  return value + (hardBreak ? "  " : "");
}

// Splits a Markdown table row into cells.
function splitTableCells(line: string): string[] | null {
  const trimmed = line.trim();
  if (!trimmed.includes("|")) return null;

  const cells: string[] = [];
  let current = "";
  let escaped = false;

  for (const character of trimmed) {
    if (escaped) {
      current += character;
      escaped = false;
      continue;
    }
    if (character === "\\") {
      current += character;
      escaped = true;
      continue;
    }
    if (character === "|") {
      cells.push(current.trim());
      current = "";
      continue;
    }

    current += character;
  }

  cells.push(current.trim());

  if (cells[0] === "") cells.shift();
  if (cells.at(-1) === "") cells.pop();

  return cells.length ? cells : null;
}

// Reads alignment from a Markdown table separator cell.
function separatorAlignment(cell: string): TableAlignment | null {
  const value = cell.trim();
  if (!/^:?-{3,}:?$/.test(value)) return null;

  return {
    left: value.startsWith(":"),
    right: value.endsWith(":"),
  };
}

// Builds a normalized Markdown table separator cell.
function tableSeparator(cells: string[] | null): TableAlignment[] | null {
  if (!cells?.length) return null;

  const alignments = cells.map(separatorAlignment);

  return alignments.every((alignment): alignment is TableAlignment =>
    Boolean(alignment),
  )
    ? alignments
    : null;
}

// Chooses the normalized separator width for a table column.
function separatorWidth(alignment: TableAlignment): number {
  if (alignment.left && alignment.right) return 5;
  if (alignment.left || alignment.right) return 4;

  return 3;
}

// Renders one Markdown table separator cell.
function renderSeparator(width: number, alignment: TableAlignment): string {
  if (alignment.left && alignment.right)
    return `:${"-".repeat(Math.max(3, width - 2))}:`;
  if (alignment.left) return `:${"-".repeat(Math.max(3, width - 1))}`;
  if (alignment.right) return `${"-".repeat(Math.max(3, width - 1))}:`;

  return "-".repeat(Math.max(3, width));
}

// Renders normalized Markdown table cells as one row.
function renderTableRow(
  cells: string[],
  widths: number[],
  indent: string,
): string {
  const padded = widths.map((width, index) =>
    (cells[index] || "").padEnd(width, " "),
  );
  return `${indent}| ${padded.join(" | ")} |`;
}

// Formats one detected Markdown table block.
function formatTable(
  lines: MarkdownLine[],
  start: number,
): FormattedTable | null {
  const first = lines[start];
  const second = lines[start + 1];
  if (!first || !second) return null;

  const header = splitTableCells(first.text);
  const separator = splitTableCells(second.text);
  const alignments = tableSeparator(separator);
  if (!header || !separator || !alignments) return null;
  if (header.length !== separator.length) return null;
  if (first.protected || second.protected) return null;

  const rows = [header];
  let end = start + 2;

  while (end < lines.length && !lines[end]?.protected) {
    const current = lines[end];
    if (!current) break;

    const cells = splitTableCells(current.text);
    if (!cells) break;
    if (cells.length > header.length) break;

    rows.push(cells);
    end += 1;
  }

  const widths = header.map((_, column) => {
    const alignment = alignments[column];
    if (!alignment) return 3;

    let width = separatorWidth(alignment);

    for (const row of rows) width = Math.max(width, (row[column] || "").length);

    return width;
  });
  const indent = first.text.match(/^ */)?.[0] || "";
  const formatted = [renderTableRow(header, widths, indent)];

  formatted.push(
    `${indent}| ${widths
      .map((width, column) =>
        renderSeparator(
          width,
          alignments[column] ?? { left: false, right: false },
        ),
      )
      .join(" | ")} |`,
  );

  for (const row of rows.slice(1))
    formatted.push(renderTableRow(row, widths, indent));

  return { lines: formatted, end };
}

// Classifies Markdown lines and normalizes safe text.
function normalizeLines(source: string): MarkdownLine[] {
  const rawLines = source
    .replaceAll("\r\n", "\n")
    .replaceAll("\r", "\n")
    .split("\n");
  const lines: MarkdownLine[] = [];
  let fence: Fence | null = null;

  for (const rawLine of rawLines) {
    if (fence) {
      lines.push({ text: rawLine, protected: true });

      if (closesFence(rawLine, fence)) fence = null;
      continue;
    }

    const opened = openingFence(rawLine);
    if (opened) {
      fence = opened;
      lines.push({ text: rawLine.replace(/[ \t]+$/, ""), protected: true });
      continue;
    }

    const protectedLine = /^( {4}|\t)/.test(rawLine);

    lines.push({
      text: normalizeEditableLine(rawLine),
      protected: protectedLine,
    });
  }

  return lines;
}

// Formats Markdown tables outside protected blocks.
function formatTables(lines: MarkdownLine[]): MarkdownLine[] {
  const result: MarkdownLine[] = [];

  for (let index = 0; index < lines.length;) {
    const table = formatTable(lines, index);
    if (!table) {
      const line = lines[index];

      if (line) result.push(line);

      index += 1;
      continue;
    }

    result.push(...table.lines.map((text) => ({ text, protected: false })));
    index = table.end;
  }

  return result;
}

// Collapses excess blank lines outside protected blocks.
function collapseBlankLines(lines: MarkdownLine[]): MarkdownLine[] {
  const result: MarkdownLine[] = [];
  let pendingBlanks = 0;

  // Flushes pending blank lines within the allowed limit.
  function flushBlanks(nextProtected: boolean): void {
    if (!pendingBlanks) return;

    const previousProtected = result.at(-1)?.protected === true;
    const count = previousProtected && nextProtected ? pendingBlanks : 1;

    for (let index = 0; index < count; index += 1)
      result.push({ text: "", protected: previousProtected && nextProtected });

    pendingBlanks = 0;
  }

  for (const line of lines) {
    if (line.text === "") {
      pendingBlanks += 1;
      continue;
    }

    flushBlanks(line.protected);
    result.push(line);
  }

  while (result[0]?.text === "") result.shift();
  while (result.at(-1)?.text === "") result.pop();

  return result;
}

// formatMarkdownDocument applies conservative, deterministic Markdown cleanup.
// It deliberately preserves fenced and indented block contents so formatting
// cannot rewrite commands, source code, Mermaid diagrams, tab bodies, or details.
export function formatMarkdownDocument(source: string): string {
  if (!source) return "";

  const lines = collapseBlankLines(formatTables(normalizeLines(source)));
  if (!lines.length) return "";

  return `${lines.map((line) => line.text).join("\n")}\n`;
}
