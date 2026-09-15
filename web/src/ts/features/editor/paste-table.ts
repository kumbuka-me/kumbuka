// Spreadsheet-style paste conversion for Markdown tables.

function escapeMarkdownCell(value: string): string {
  return value.trim().replaceAll("\\", "\\\\").replaceAll("|", "\\|");
}

// Converts tabular clipboard text into a Markdown table.
export function tabularTextToMarkdown(value: string): string | null {
  const normalized = value
    .replaceAll("\r\n", "\n")
    .replaceAll("\r", "\n")
    .trimEnd();
  const rows = normalized.split("\n").map((line) => line.split("\t"));
  if (rows.length < 2) return null;

  const columns = Math.max(...rows.map((row) => row.length));
  if (columns < 2) return null;
  if (!rows.every((row) => row.length === columns)) return null;

  const cells = rows.map((row) => row.map(escapeMarkdownCell));
  // Renders one Markdown table row.
  const line = (row: string[]): string => `| ${row.join(" | ")} |`;
  const separator = line(Array.from({ length: columns }, () => "---"));

  return [line(cells[0]), separator, ...cells.slice(1).map(line)].join("\n");
}

// Wires table paste behavior.
function setupTablePaste(form: HTMLFormElement): void {
  const textarea = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  if (!textarea) return;

  textarea.addEventListener("paste", (event: ClipboardEvent) => {
    if ((event.clipboardData?.files?.length || 0) > 0) return;

    const text = event.clipboardData?.getData("text/plain") || "";
    const markdown = tabularTextToMarkdown(text);
    if (!markdown) return;

    event.preventDefault();

    const start = textarea.selectionStart ?? textarea.value.length;
    const end = textarea.selectionEnd ?? start;

    textarea.setRangeText(markdown, start, end, "end");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

// Initializes table paste.
export function initTablePaste(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupTablePaste(form);
}
