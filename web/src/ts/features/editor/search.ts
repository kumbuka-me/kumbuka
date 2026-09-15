// In-editor plain-text search and replacement.

interface ReplaceResult {
  value: string;
  count: number;
}

export function replaceAllPlainText(
  source: string,
  query: string,
  replacement: string,
  matchCase = false,
): ReplaceResult {
  if (!query) return { value: source, count: 0 };

  const haystack = matchCase ? source : source.toLocaleLowerCase();
  const needle = matchCase ? query : query.toLocaleLowerCase();
  let cursor = 0;
  let count = 0;
  let value = "";

  while (cursor < source.length) {
    const index = haystack.indexOf(needle, cursor);
    if (index < 0) break;

    value += source.slice(cursor, index) + replacement;
    cursor = index + query.length;
    count += 1;
  }

  if (!count) return { value: source, count: 0 };

  value += source.slice(cursor);
  return { value, count };
}

// Wires editor search behavior.
function setupEditorSearch(form: HTMLFormElement): void {
  const panel = form.querySelector<HTMLElement>("[data-editor-search]");
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const find = panel?.querySelector<HTMLInputElement>("[data-editor-find]");
  const replacement = panel?.querySelector<HTMLInputElement>(
    "[data-editor-replace]",
  );
  const caseToggle = panel?.querySelector<HTMLInputElement>(
    "[data-editor-find-case]",
  );
  const status = panel?.querySelector<HTMLElement>("[data-editor-find-status]");
  if (!panel || !source || !find || !replacement || !caseToggle || !status)
    return;

  const searchPanel = panel;
  const sourceEditor = source;
  const findInput = find;
  const replaceInput = replacement;
  const matchCaseInput = caseToggle;
  const statusElement = status;

  // Normalizes text according to the current case mode.
  function normalized(value: string): string {
    return matchCaseInput.checked ? value : value.toLocaleLowerCase();
  }

  // Finds match.
  function findMatch(direction = 1): boolean {
    const query = findInput.value;
    if (!query) {
      statusElement.textContent = "Enter text to find.";
      return false;
    }

    const haystack = normalized(sourceEditor.value);
    const needle = normalized(query);
    let index: number;

    if (direction < 0) {
      const from = Math.max(0, (sourceEditor.selectionStart ?? 0) - 1);

      index = haystack.lastIndexOf(needle, from);

      if (index < 0) index = haystack.lastIndexOf(needle);
    } else {
      const from = sourceEditor.selectionEnd ?? 0;

      index = haystack.indexOf(needle, from);

      if (index < 0) index = haystack.indexOf(needle);
    }

    if (index < 0) {
      statusElement.textContent = "No matches.";
      return false;
    }

    sourceEditor.focus();
    sourceEditor.setSelectionRange(index, index + query.length);

    const before = sourceEditor.value.slice(0, index);
    const line = before.split("\n").length;

    statusElement.textContent = `Match on line ${line}.`;
    return true;
  }

  // Opens the editor search panel.
  function open(): void {
    searchPanel.hidden = false;

    const selected = sourceEditor.value.slice(
      sourceEditor.selectionStart ?? 0,
      sourceEditor.selectionEnd ?? 0,
    );

    if (selected && !selected.includes("\n")) findInput.value = selected;

    requestAnimationFrame(() => {
      findInput.focus();
      findInput.select();
    });
  }

  // Closes the editor search panel.
  function close(): void {
    searchPanel.hidden = true;
    sourceEditor.focus();
  }

  form.addEventListener("editor:find", open);
  searchPanel
    .querySelector<HTMLButtonElement>("[data-editor-find-close]")
    ?.addEventListener("click", close);
  searchPanel
    .querySelector<HTMLButtonElement>("[data-editor-find-next]")
    ?.addEventListener("click", () => findMatch(1));
  searchPanel
    .querySelector<HTMLButtonElement>("[data-editor-find-previous]")
    ?.addEventListener("click", () => findMatch(-1));
  searchPanel
    .querySelector<HTMLButtonElement>("[data-editor-replace-one]")
    ?.addEventListener("click", () => {
      const selected = sourceEditor.value.slice(
        sourceEditor.selectionStart ?? 0,
        sourceEditor.selectionEnd ?? 0,
      );

      if (normalized(selected) !== normalized(findInput.value)) {
        if (!findMatch(1)) return;
      }

      sourceEditor.setRangeText(
        replaceInput.value,
        sourceEditor.selectionStart ?? 0,
        sourceEditor.selectionEnd ?? 0,
        "end",
      );
      sourceEditor.dispatchEvent(new Event("input", { bubbles: true }));
      statusElement.textContent = "Replaced 1 match.";
    });
  searchPanel
    .querySelector<HTMLButtonElement>("[data-editor-replace-all]")
    ?.addEventListener("click", () => {
      const result = replaceAllPlainText(
        sourceEditor.value,
        findInput.value,
        replaceInput.value,
        matchCaseInput.checked,
      );

      if (result.count) {
        sourceEditor.value = result.value;
        sourceEditor.dispatchEvent(new Event("input", { bubbles: true }));
      }

      statusElement.textContent = `Replaced ${result.count} match${result.count === 1 ? "" : "es"}.`;
    });

  findInput.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key === "Enter") {
      event.preventDefault();
      findMatch(event.shiftKey ? -1 : 1);
    } else if (event.key === "Escape") {
      close();
    }
  });

  sourceEditor.addEventListener("keydown", (event: KeyboardEvent) => {
    if (
      (event.ctrlKey || event.metaKey) &&
      event.key.toLocaleLowerCase() === "f"
    ) {
      event.preventDefault();
      open();
    }
  });
}

// Initializes editor search.
export function initEditorSearch(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupEditorSearch(form);
}
