// Markdown list continuation for the plain-text editor.

export interface MarkdownListEdit {
  start: number;
  end: number;
  text: string;
  caret: number;
}

type MarkdownListMarker = UnorderedListMarker | OrderedListMarker;

interface BaseListMarker {
  content: string;
  indent: string;
  nextPrefix: string;
}

interface UnorderedListMarker extends BaseListMarker {
  kind: "unordered";
}

interface OrderedListMarker extends BaseListMarker {
  kind: "ordered";
  delimiter: "." | ")";
  number: number;
  numberEnd: number;
  numberStart: number;
}

interface RenumberedTail {
  end: number;
  text: string;
}

function isListWhitespace(character: string | undefined): boolean {
  return character === " " || character === "\t";
}

function skipListWhitespace(value: string, start: number): number {
  let index = start;

  while (isListWhitespace(value[index])) index++;

  return index;
}

// Returns the opening fence marker at the start of a Markdown line, if present.
function markdownFenceMarker(line: string): string {
  let index = skipListWhitespace(line, 0);
  const marker = line[index];

  if (marker !== "`" && marker !== "~") return "";

  const start = index;
  while (line[index] === marker) index++;

  if (index - start < 3) return "";

  return line.slice(start, index);
}

// Reports whether the caret is currently inside a fenced Markdown code block.
function fencedCodeAt(value: string, caret: number): boolean {
  const lines = value.slice(0, caret).split("\n");
  let fence = "";

  for (const line of lines) {
    const marker = markdownFenceMarker(line);
    if (!marker) continue;

    if (!fence) {
      fence = marker;
      continue;
    }

    if (marker[0] === fence[0] && marker.length >= fence.length) fence = "";
  }

  return fence !== "";
}

function taskContentStart(line: string, start: number): number | null {
  if (
    line[start] !== "[" ||
    (line[start + 1] !== " " &&
      line[start + 1] !== "x" &&
      line[start + 1] !== "X") ||
    line[start + 2] !== "]"
  )
    return null;

  return skipListWhitespace(line, start + 3);
}

function unorderedListMarker(
  line: string,
  indent: string,
  start: number,
): UnorderedListMarker | null {
  const bullet = line[start];
  if (bullet !== "-" && bullet !== "+" && bullet !== "*") return null;

  const contentStart = skipListWhitespace(line, start + 1);
  if (contentStart === start + 1) return null;

  const taskStart = taskContentStart(line, contentStart);
  if (taskStart !== null) {
    return {
      kind: "unordered",
      content: line.slice(taskStart),
      indent,
      nextPrefix: `${indent}${bullet} [ ] `,
    };
  }

  return {
    kind: "unordered",
    content: line.slice(contentStart),
    indent,
    nextPrefix: `${indent}${bullet} `,
  };
}

function orderedListMarker(
  line: string,
  indent: string,
  start: number,
): OrderedListMarker | null {
  let index = start;

  while (line[index] >= "0" && line[index] <= "9") index++;
  if (index === start) return null;

  const delimiter = line[index];
  if (delimiter !== "." && delimiter !== ")") return null;

  const contentStart = skipListWhitespace(line, index + 1);
  if (contentStart === index + 1) return null;

  const number = Number.parseInt(line.slice(start, index), 10);

  return {
    kind: "ordered",
    content: line.slice(contentStart),
    delimiter,
    indent,
    number,
    numberEnd: index,
    numberStart: start,
    nextPrefix: `${indent}${number + 1}${delimiter} `,
  };
}

// Parses a Markdown list marker and returns the prefix for the next item.
function markdownListMarker(line: string): MarkdownListMarker | null {
  const markerStart = skipListWhitespace(line, 0);
  const indent = line.slice(0, markerStart);

  return (
    unorderedListMarker(line, indent, markerStart) ??
    orderedListMarker(line, indent, markerStart)
  );
}

function sameListIndent(
  line: string,
  indent: string,
): "same" | "deeper" | "outside" {
  const whitespaceEnd = skipListWhitespace(line, 0);
  const lineIndent = line.slice(0, whitespaceEnd);

  if (lineIndent === indent) return "same";
  if (lineIndent.length > indent.length) return "deeper";

  return "outside";
}

// Renumbers following siblings in one ordered list while leaving nested content unchanged.
function renumberOrderedTail(
  value: string,
  start: number,
  indent: string,
  delimiter: "." | ")",
  nextNumber: number,
): RenumberedTail {
  let cursor = start;
  let expected = nextNumber;
  let text = "";

  while (cursor < value.length && value[cursor] === "\n") {
    const lineStart = cursor + 1;
    const nextNewline = value.indexOf("\n", lineStart);
    const lineEnd = nextNewline === -1 ? value.length : nextNewline;
    const line = value.slice(lineStart, lineEnd);

    if (!line.trim()) {
      text += value.slice(cursor, lineEnd);
      cursor = lineEnd;
      continue;
    }

    const indentRelation = sameListIndent(line, indent);
    const marker = markdownListMarker(line);

    if (indentRelation === "outside") break;

    if (indentRelation === "deeper") {
      text += value.slice(cursor, lineEnd);
      cursor = lineEnd;
      continue;
    }

    if (!marker || marker.kind !== "ordered" || marker.delimiter !== delimiter)
      break;

    text +=
      value.slice(cursor, lineStart) +
      line.slice(0, marker.numberStart) +
      expected +
      line.slice(marker.numberEnd);
    expected++;
    cursor = lineEnd;
  }

  return { end: cursor, text };
}

// Returns the edit needed when Enter should continue or exit a Markdown list.
export function markdownListEnterEdit(
  value: string,
  caret: number,
): MarkdownListEdit | null {
  if (caret < 0 || caret > value.length || fencedCodeAt(value, caret))
    return null;

  const lineStart = value.lastIndexOf("\n", caret - 1) + 1;
  const nextNewline = value.indexOf("\n", caret);
  const lineEnd = nextNewline === -1 ? value.length : nextNewline;

  // Only continue lists when Enter is pressed at the end of the current line.
  if (caret !== lineEnd) return null;

  const line = value.slice(lineStart, lineEnd);
  const marker = markdownListMarker(line);
  if (!marker) return null;

  if (!marker.content.trim()) {
    if (marker.kind === "ordered") {
      const tail = renumberOrderedTail(
        value,
        lineEnd,
        marker.indent,
        marker.delimiter,
        marker.number,
      );

      return {
        start: lineStart,
        end: tail.end,
        text: tail.text,
        caret: lineStart,
      };
    }

    return {
      start: lineStart,
      end: lineEnd,
      text: "",
      caret: lineStart,
    };
  }

  const prefix = `\n${marker.nextPrefix}`;

  if (marker.kind === "ordered") {
    const tail = renumberOrderedTail(
      value,
      lineEnd,
      marker.indent,
      marker.delimiter,
      marker.number + 2,
    );

    return {
      start: caret,
      end: tail.end,
      text: prefix + tail.text,
      caret: caret + prefix.length,
    };
  }

  return {
    start: caret,
    end: caret,
    text: prefix,
    caret: caret + prefix.length,
  };
}

function setupMarkdownListContinuation(source: HTMLTextAreaElement): void {
  source.addEventListener("keydown", (event: KeyboardEvent) => {
    if (
      event.defaultPrevented ||
      event.key !== "Enter" ||
      event.shiftKey ||
      event.ctrlKey ||
      event.metaKey ||
      event.altKey ||
      event.isComposing ||
      source.selectionStart !== source.selectionEnd
    )
      return;

    const caret = source.selectionStart ?? source.value.length;
    const edit = markdownListEnterEdit(source.value, caret);
    if (!edit) return;

    event.preventDefault();
    source.setRangeText(edit.text, edit.start, edit.end, "start");
    source.setSelectionRange(edit.caret, edit.caret);
    source.dispatchEvent(new Event("input", { bubbles: true }));
  });
}

// Initializes automatic continuation and exit behavior for Markdown lists.
export function initMarkdownListContinuation(): void {
  for (const source of document.querySelectorAll<HTMLTextAreaElement>(
    "textarea[data-markdown-editor]",
  ))
    setupMarkdownListContinuation(source);
}
