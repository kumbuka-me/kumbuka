// Generic plugin-owned editor completion behavior.

import { textareaCaretOffset } from "../../core/textarea.ts";
import { loadEditorCatalog, type CatalogCompletion } from "./catalog.ts";

type CompletionTrigger = { start: number; query: string; trigger: string };
type Fence = { character: string; length: number };

function fencedCodeAt(value: string, caret: number): boolean {
  const lines = value.slice(0, caret).split("\n");
  let fence: Fence | null = null;

  for (const line of lines) {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})/u);
    if (!match) continue;

    const marker = match[1];
    const next: Fence = { character: marker[0], length: marker.length };
    if (!fence) {
      fence = next;
      continue;
    }
    if (fence.character === next.character && next.length >= fence.length)
      fence = null;
  }
  return fence !== null;
}

export function completionTrigger(
  value: string,
  caret: number,
  triggers: string[],
): CompletionTrigger | null {
  if (fencedCodeAt(value, caret)) return null;

  const lineStart = value.lastIndexOf("\n", Math.max(0, caret - 1)) + 1;
  const fragment = value.slice(lineStart, caret);
  let selected = "";
  let opening = -1;

  for (const trigger of triggers) {
    const index = fragment.lastIndexOf(trigger);
    if (
      index > opening ||
      (index === opening && trigger.length > selected.length)
    ) {
      opening = index;
      selected = trigger;
    }
  }
  if (opening < 0 || !selected) return null;

  const query = fragment.slice(opening + selected.length);
  if (/[{}]/u.test(query)) return null;
  return { start: lineStart + opening, query, trigger: selected };
}

function matches(
  items: CatalogCompletion[],
  trigger: CompletionTrigger,
): CatalogCompletion[] {
  const normalized = trigger.query.trim().toLocaleLowerCase();
  return items
    .filter((item) => item.trigger === trigger.trigger)
    .filter(
      (item) =>
        !normalized ||
        `${item.label} ${item.detail ?? ""}`
          .toLocaleLowerCase()
          .includes(normalized),
    );
}

function setupPluginCompletions(source: HTMLTextAreaElement): void {
  const anchor = source.parentElement;
  if (!anchor) return;
  const completionAnchor = anchor;

  const menu = document.createElement("div");
  menu.className = "editor-suggestion-menu editor-plugin-completion-menu";
  menu.id = "plugin-completion-suggestions";
  menu.hidden = true;
  menu.setAttribute("role", "listbox");
  menu.setAttribute("aria-label", "Insert reusable content");
  completionAnchor.append(menu);

  let catalog: CatalogCompletion[] | null = null;
  let load: Promise<CatalogCompletion[]> | null = null;
  let trigger: CompletionTrigger | null = null;
  let results: CatalogCompletion[] = [];
  let active = -1;
  let request = 0;

  function close(): void {
    request += 1;
    trigger = null;
    results = [];
    active = -1;
    menu.hidden = true;
    menu.replaceChildren();
    if (source.getAttribute("aria-controls") === menu.id) {
      source.removeAttribute("aria-controls");
      source.setAttribute("aria-expanded", "false");
    }
  }

  function position(): void {
    if (menu.hidden) return;
    const sourceRect = source.getBoundingClientRect();
    const anchorRect = completionAnchor.getBoundingClientRect();
    const caret = textareaCaretOffset(source, source.selectionStart ?? 0);
    const menuWidth = Math.min(
      390,
      Math.max(260, completionAnchor.clientWidth - 16),
    );
    const maxLeft = Math.max(8, completionAnchor.clientWidth - menuWidth - 8);
    const rawLeft = sourceRect.left - anchorRect.left + caret.left;
    menu.style.width = `${menuWidth}px`;
    menu.style.left = `${Math.max(8, Math.min(rawLeft, maxLeft))}px`;
    menu.style.top = `${sourceRect.top - anchorRect.top + caret.top}px`;
  }

  function render(): void {
    menu.replaceChildren();
    if (!results.length) {
      const empty = document.createElement("div");
      empty.className = "editor-suggestion-empty";
      empty.textContent = "No matching items.";
      menu.append(empty);
    } else {
      for (const [index, item] of results.entries()) {
        const option = document.createElement("button");
        option.type = "button";
        option.className = "editor-suggestion-option";
        option.dataset.pluginCompletionIndex = String(index);
        option.setAttribute("role", "option");
        option.setAttribute("aria-selected", String(index === active));
        const name = document.createElement("strong");
        const detail = document.createElement("small");
        name.textContent = item.label;
        detail.textContent = item.detail || item.replacement;
        option.append(name, detail);
        menu.append(option);
      }
    }
    menu.hidden = false;
    source.setAttribute("aria-controls", menu.id);
    source.setAttribute("aria-expanded", "true");
    position();
  }

  function choose(index: number): void {
    const item = results[index];
    const current = trigger;
    if (!item || !current) return;
    const end = source.selectionStart ?? current.start;
    source.setRangeText(item.replacement, current.start, end, "end");
    source.dispatchEvent(new Event("input", { bubbles: true }));
    close();
    source.focus();
  }

  async function loadItems(): Promise<CatalogCompletion[]> {
    if (catalog) return catalog;
    if (load) return load;
    load = loadEditorCatalog()
      .then((catalog) => catalog.completions)
      .then((items) => {
        catalog = items;
        return items;
      })
      .finally(() => {
        load = null;
      });
    return load;
  }

  async function refresh(): Promise<void> {
    try {
      const available = await loadItems();
      const triggers = [...new Set(available.map((item) => item.trigger))].sort(
        (left, right) => right.length - left.length,
      );
      const next = completionTrigger(
        source.value,
        source.selectionStart ?? 0,
        triggers,
      );
      if (!next) {
        close();
        return;
      }
      trigger = next;
      const currentRequest = ++request;
      results = matches(available, next);
      if (currentRequest !== request) return;
      active = results.length ? 0 : -1;
      render();
    } catch (error) {
      console.error("plugin completion catalog failed", error);
      close();
    }
  }

  source.addEventListener("input", () => void refresh());
  source.addEventListener("click", () => void refresh());
  source.addEventListener("scroll", position);
  source.addEventListener("keydown", (event: KeyboardEvent) => {
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
          choose(active);
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
    const option = target.closest<HTMLElement>(
      "[data-plugin-completion-index]",
    );
    if (option) choose(Number(option.dataset.pluginCompletionIndex));
  });
  document.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (target !== source && target instanceof Node && !menu.contains(target))
      close();
  });
  window.addEventListener("resize", position);
}

// Initializes active plugin-owned completions in Markdown editors.
export function initPluginCompletions(): void {
  for (const source of document.querySelectorAll<HTMLTextAreaElement>(
    "textarea[data-plugin-completions]",
  ))
    setupPluginCompletions(source);
}
