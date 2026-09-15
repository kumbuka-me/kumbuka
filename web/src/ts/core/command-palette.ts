// Global command palette search and keyboard interaction.

import { requiredElement } from "./dom.ts";
import { isRecord, requireArrayOf } from "./guards.ts";
import { requestJSON } from "./http.ts";

const maxResults = 7;

interface CommandPage {
  slug: string;
  title: string;
}

function isCommandPage(value: unknown): value is CommandPage {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    typeof value.title === "string"
  );
}

// Wires command-palette search, navigation, and actions.
function setupCommandPalette(dialog: HTMLDialogElement): void {
  const inputElement = requiredElement<HTMLInputElement>(
    dialog,
    "[data-command-palette-input]",
  );
  const resultsElement = requiredElement<HTMLElement>(
    dialog,
    "[data-command-palette-results]",
  );
  const closeButton = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-command-palette-close]",
  );

  let pages: CommandPage[] = [];
  let active = -1;
  let requestID = 0;

  // Returns the currently visible command-palette options.
  function allOptions(): HTMLElement[] {
    return [
      ...dialog.querySelectorAll<HTMLElement>(
        "[data-command-option]:not([hidden])",
      ),
    ];
  }

  // Sets active.
  function setActive(index: number): void {
    const options = allOptions();
    if (!options.length) {
      active = -1;
      return;
    }

    active = ((index % options.length) + options.length) % options.length;
    options.forEach((option, optionIndex) =>
      option.setAttribute("aria-selected", String(optionIndex === active)),
    );
    options[active]?.scrollIntoView({ block: "nearest" });
  }

  // Renders pages.
  function renderPages(): void {
    resultsElement.replaceChildren();
    pages.forEach((page) => {
      const anchor = document.createElement("a");

      anchor.href = `/pages/${page.slug}`;
      anchor.dataset.commandOption = "";
      anchor.className = "command-palette-option";
      anchor.setAttribute("role", "option");

      const strong = document.createElement("strong");
      const small = document.createElement("small");

      strong.textContent = page.title;
      small.textContent = page.slug;
      anchor.append(strong, small);
      resultsElement.append(anchor);
    });
    setActive(0);
  }

  // Runs the current command-palette search.
  async function search(): Promise<void> {
    const query = inputElement.value.trim();
    if (!query) {
      pages = [];
      resultsElement.replaceChildren();

      for (const option of dialog.querySelectorAll<HTMLElement>(
        "[data-command-static]",
      ))
        option.hidden = false;

      setActive(0);
      return;
    }

    for (const option of dialog.querySelectorAll<HTMLElement>(
      "[data-command-static]",
    )) {
      option.hidden = !(option.textContent ?? "")
        .toLocaleLowerCase()
        .includes(query.toLocaleLowerCase());
    }

    const current = ++requestID;

    try {
      const value = await requestJSON(
        `/api/search?q=${encodeURIComponent(query)}`,
      );
      if (current !== requestID) return;

      pages = requireArrayOf(value, isCommandPage, "search response").slice(
        0,
        maxResults,
      );
      renderPages();
    } catch (error) {
      console.error("command palette search failed", error);
      pages = [];
      renderPages();
    }
  }

  // Opens the command palette and resets its state.
  function open(): void {
    if (!dialog.open) dialog.showModal();

    inputElement.value = "";
    pages = [];
    resultsElement.replaceChildren();

    for (const option of dialog.querySelectorAll<HTMLElement>(
      "[data-command-static]",
    ))
      option.hidden = false;

    setActive(0);
    requestAnimationFrame(() => inputElement.focus());
  }

  // Closes the command palette.
  function close(): void {
    if (dialog.open) dialog.close();
  }

  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (isCommandPaletteShortcut(event)) {
      event.preventDefault();
      open();
      return;
    }
    if (!dialog.open) return;

    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        setActive(active + 1);
        break;
      case "ArrowUp":
        event.preventDefault();
        setActive(active - 1);
        break;
      case "Enter": {
        const option = allOptions()[active];
        if (option) {
          event.preventDefault();
          option.click();
        }
        break;
      }
    }
  });

  inputElement.addEventListener("input", () => void search());
  closeButton.addEventListener("click", close);
  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) close();
  });
}

function isCommandPaletteShortcut(event: KeyboardEvent): boolean {
  if (event.shiftKey) return false;
  if (!event.ctrlKey && !event.metaKey) return false;

  return event.key.toLocaleLowerCase() === "k";
}

// Initializes command palette.
export function initCommandPalette(): void {
  const dialog = document.querySelector<HTMLDialogElement>(
    "[data-command-palette]",
  );
  if (dialog) setupCommandPalette(dialog);
}
