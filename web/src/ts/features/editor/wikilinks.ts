// Wiki-link autocomplete in the Markdown editor.

import { isRecord, requireArrayOf } from "../../core/guards.ts";
import { requestJSON } from "../../core/http.ts";

const resultLimit = 8;

interface WikiLinkTrigger {
  start: number;
  query: string;
}

interface WikiLinkPage {
  slug: string;
  title: string;
}

function isWikiLinkPage(value: unknown): value is WikiLinkPage {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    typeof value.title === "string"
  );
}

// Finds an unfinished wiki link at the caret.
export function wikiLinkTrigger(
  value: string,
  caret: number,
): WikiLinkTrigger | null {
  const before = value.slice(0, caret);
  const open = before.lastIndexOf("[[");
  if (open < 0) return null;

  const body = before.slice(open + 2);
  if (body.includes("]]")) return null;
  if (body.includes("\n")) return null;

  const query = body.split("|", 1)[0]?.trim() ?? "";

  return { start: open, query };
}

// Builds the canonical wiki-link replacement text.
export function wikiLinkReplacement(title: string): string {
  return `[[${title}]]`;
}

// Creates menu.
function createMenu(source: HTMLTextAreaElement): HTMLDivElement {
  const menu = document.createElement("div");

  menu.className = "editor-suggestion-menu";
  menu.dataset.editorWikiLinks = "";
  menu.hidden = true;
  menu.setAttribute("role", "listbox");
  menu.setAttribute("aria-label", "Wiki link suggestions");
  source.closest<HTMLElement>(".editor-source-pane")?.append(menu);
  return menu;
}

// Sets menu position.
function setMenuPosition(source: HTMLTextAreaElement, menu: HTMLElement): void {
  const pane = source.closest<HTMLElement>(".editor-source-pane");
  if (!pane) return;

  const sourceRect = source.getBoundingClientRect();
  const paneRect = pane.getBoundingClientRect();

  menu.style.left = `${Math.max(12, sourceRect.left - paneRect.left + 18)}px`;
  menu.style.top = `${Math.max(12, sourceRect.top - paneRect.top + 52)}px`;
}

// Wires wiki links behavior.
function setupWikiLinks(form: HTMLFormElement): void {
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  if (!source) return;

  const editor = source;

  const menu = createMenu(editor);
  let trigger: WikiLinkTrigger | null = null;
  let results: WikiLinkPage[] = [];
  let active = -1;
  let request = 0;

  // Closes wiki-link suggestions.
  function close(): void {
    menu.hidden = true;
    menu.replaceChildren();
    active = -1;
    results = [];
    trigger = null;
    editor.setAttribute("aria-expanded", "false");
  }

  // Renders wiki-link suggestions.
  function render(): void {
    menu.replaceChildren();

    if (!results.length) {
      const empty = document.createElement("div");

      empty.className = "editor-suggestion-empty";
      empty.textContent = "No matching pages.";
      menu.append(empty);
    } else {
      results.forEach((page, index) => {
        const button = document.createElement("button");

        button.type = "button";
        button.className = "editor-suggestion-option";
        button.dataset.index = String(index);
        button.setAttribute("role", "option");
        button.setAttribute("aria-selected", String(index === active));

        const strong = document.createElement("strong");
        const small = document.createElement("small");

        strong.textContent = page.title;
        small.textContent = page.slug;
        button.append(strong, small);
        menu.append(button);
      });
    }

    setMenuPosition(editor, menu);
    menu.hidden = false;
    editor.setAttribute("aria-expanded", "true");
  }

  // Inserts the selected wiki-link suggestion.
  function select(index: number): void {
    const page = results[index];
    const currentTrigger = trigger;
    if (!page || !currentTrigger) return;

    const end = editor.selectionStart ?? currentTrigger.start;
    const replacement = wikiLinkReplacement(page.title);

    editor.setRangeText(replacement, currentTrigger.start, end, "end");
    editor.dispatchEvent(new Event("input", { bubbles: true }));
    close();
    editor.focus();
  }

  // Refreshes wiki-link suggestions at the caret.
  async function refresh(): Promise<void> {
    const caret = editor.selectionStart ?? 0;
    const next = wikiLinkTrigger(editor.value, caret);
    if (!next) {
      close();
      return;
    }

    trigger = next;

    const currentRequest = ++request;

    try {
      const payload = await requestJSON(
        `/api/search?q=${encodeURIComponent(next.query)}`,
      );
      if (currentRequest !== request) return;

      results = requireArrayOf(
        payload,
        isWikiLinkPage,
        "wiki-link search response",
      ).slice(0, resultLimit);
      active = results.length ? 0 : -1;
      render();
    } catch (error) {
      console.error("wiki-link search failed", error);
      close();
    }
  }

  editor.addEventListener("input", () => void refresh());
  editor.addEventListener("click", () => void refresh());
  editor.addEventListener("keydown", (event: KeyboardEvent) => {
    if (menu.hidden) return;
    switch (event.key) {
      case "ArrowDown":
        if (results.length) {
          event.preventDefault();
          active = (active + 1) % results.length;
          render();
        }
        break;
      case "ArrowUp":
        if (results.length) {
          event.preventDefault();
          active = (active - 1 + results.length) % results.length;
          render();
        }
        break;
      case "Enter":
      case "Tab":
        if (active >= 0) {
          event.preventDefault();
          select(active);
        }
        break;
      case "Escape":
        event.preventDefault();
        close();
        break;
    }
  });

  menu.addEventListener("mousedown", (event: MouseEvent) =>
    event.preventDefault(),
  );
  menu.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const option = target.closest<HTMLElement>("[data-index]");

    if (option) select(Number(option.dataset.index));
  });

  document.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Node)) return;

    if (target !== editor && !menu.contains(target)) close();
  });
}

// Initializes wiki link autocomplete.
export function initWikiLinkAutocomplete(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupWikiLinks(form);
}
