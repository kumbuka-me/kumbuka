// Editor slash-command matching and menu behavior.

import { requestJSON } from "../../core/http.ts";
import { parseEditorCatalog, type CatalogInsert } from "./catalog.ts";
import { applyEditorInsertAction, insertInlineAtSelection } from "./toolbar.ts";

interface SlashCommand {
  id: string;
  label: string;
  description: string;
  markdown?: string;
  insert?: CatalogInsert;
  action?: "table" | "image" | "mention";
}

interface SlashCommandTrigger {
  start: number;
  query: string;
}

const commands: SlashCommand[] = [
  {
    id: "heading-1",
    label: "Heading 1",
    description: "Large section heading",
    markdown: "# Heading",
  },
  {
    id: "heading-2",
    label: "Heading 2",
    description: "Section heading",
    markdown: "## Heading",
  },
  {
    id: "heading-3",
    label: "Heading 3",
    description: "Subsection heading",
    markdown: "### Heading",
  },
  {
    id: "code",
    label: "Code block",
    description: "Fenced code block",
    markdown: "```\ncode\n```",
  },
  {
    id: "callout",
    label: "Callout",
    description: "Highlighted note block",
    markdown: "!!! note\nImportant information.",
  },
  {
    id: "table",
    label: "Table",
    description: "Insert or format a Markdown table",
    action: "table",
  },
  {
    id: "tabs",
    label: "Tabs",
    description: "Tabbed content",
    markdown:
      '=== "Tab 1"\n\n    First tab content.\n\n=== "Tab 2"\n\n    Second tab content.',
  },
  {
    id: "details",
    label: "Details",
    description: "Collapsible details block",
    markdown: '??? "Details"\n\n    Hidden details.',
  },
  {
    id: "mention",
    label: "Mention",
    description: "Mention a Kumbuka user",
    action: "mention",
  },
  {
    id: "image",
    label: "Image",
    description: "Upload or choose an image",
    action: "image",
  },
  {
    id: "subpages",
    label: "Subpages",
    description: "Insert child-page navigation",
    markdown: "{{subpages}}",
  },
];

// Finds an active slash-command trigger at the caret.
export function slashCommandTrigger(
  value: string,
  caret: number,
): SlashCommandTrigger | null {
  const lineStart = value.lastIndexOf("\n", Math.max(0, caret - 1)) + 1;
  const fragment = value.slice(lineStart, caret);
  const match = fragment.match(/^\s*\/([^\s]*)$/u);
  if (!match) return null;

  return { start: lineStart, query: (match[1] ?? "").toLocaleLowerCase() };
}

// Filters slash commands by label and description.
function filterCommands(items: SlashCommand[], query: string): SlashCommand[] {
  const normalized = query.trim().toLocaleLowerCase();
  if (!normalized) return items;

  return items.filter((command) =>
    `${command.label} ${command.description} ${command.id}`
      .toLocaleLowerCase()
      .includes(normalized),
  );
}

// Returns slash commands matching a query.
export function matchingSlashCommands(query: string): SlashCommand[] {
  return filterCommands(commands, query);
}

// Wires slash commands behavior.
function setupSlashCommands(form: HTMLFormElement): void {
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const pane = source?.closest<HTMLElement>(".editor-source-pane");
  if (!source || !pane) return;

  const editor = source;
  let availableCommands: SlashCommand[] = [...commands];

  async function loadReusableCommands(): Promise<void> {
    try {
      const catalog = parseEditorCatalog(
        await requestJSON("/api/editor/catalog"),
      );
      const inserts: SlashCommand[] = catalog.inserts.map((item) => ({
        id: `${item.plugin_id}-${item.module_id}`,
        label: item.name,
        description: item.description || "Insert plugin content",
        insert: item,
      }));
      const completions: SlashCommand[] = catalog.completions.map((item) => ({
        id: `${item.plugin_id}-${item.module_id}-${item.label}`,
        label: item.label,
        description: item.detail || "Insert reusable content",
        markdown: item.replacement,
      }));
      availableCommands = [...commands, ...inserts, ...completions];
    } catch {
      // Plugin and reusable commands are optional; keep built-in commands.
    }
  }

  void loadReusableCommands();

  const menu = document.createElement("div");

  menu.className = "editor-suggestion-menu editor-command-menu";
  menu.hidden = true;
  menu.setAttribute("role", "listbox");
  menu.setAttribute("aria-label", "Editor commands");
  pane.append(menu);

  let trigger: SlashCommandTrigger | null = null;
  let matches: SlashCommand[] = [];
  let active = 0;

  // Closes the slash-command menu.
  function close(): void {
    menu.hidden = true;
    menu.replaceChildren();
    trigger = null;
    matches = [];
  }

  // Renders matching slash-command options.
  function render(): void {
    menu.replaceChildren();
    matches.forEach((command, index) => {
      const button = document.createElement("button");

      button.type = "button";
      button.className = "editor-suggestion-option";
      button.dataset.commandIndex = String(index);
      button.setAttribute("role", "option");
      button.setAttribute("aria-selected", String(index === active));

      const strong = document.createElement("strong");
      const small = document.createElement("small");

      strong.textContent = command.label;
      small.textContent = command.description;
      button.append(strong, small);
      menu.append(button);
    });

    if (!matches.length) {
      const empty = document.createElement("div");

      empty.className = "editor-suggestion-empty";
      empty.textContent = "No matching commands.";
      menu.append(empty);
    }

    menu.style.left = "18px";
    menu.style.top = "52px";
    menu.hidden = false;
  }

  // Applies the selected slash command.
  function choose(index: number): void {
    const command = matches[index];
    const currentTrigger = trigger;
    if (!command || !currentTrigger) return;

    const end = editor.selectionStart ?? currentTrigger.start;

    if (command.insert) {
      editor.setRangeText("", currentTrigger.start, end, "end");
      applyEditorInsertAction(editor, command.insert);
    } else if (command.markdown) {
      editor.setRangeText(command.markdown, currentTrigger.start, end, "end");
      editor.dispatchEvent(new Event("input", { bubbles: true }));
      editor.focus();
    } else {
      switch (command.action) {
        case "table":
          editor.setRangeText("", currentTrigger.start, end, "end");
          editor.dispatchEvent(new Event("input", { bubbles: true }));
          form.querySelector<HTMLElement>("[data-table-format-open]")?.click();
          break;
        case "image":
          editor.setRangeText("", currentTrigger.start, end, "end");
          editor.dispatchEvent(new Event("input", { bubbles: true }));
          form.querySelector<HTMLElement>("[data-media-dialog-open]")?.click();
          break;
        case "mention":
          editor.setRangeText("", currentTrigger.start, end, "end");
          insertInlineAtSelection(editor, "@");
          break;
      }
    }

    close();
  }

  // Refreshes slash-command matches at the caret.
  function refresh(): void {
    const next = slashCommandTrigger(editor.value, editor.selectionStart ?? 0);
    if (!next) {
      close();
      return;
    }

    trigger = next;
    matches = filterCommands(availableCommands, next.query);
    active = 0;
    render();
  }

  editor.addEventListener("input", refresh);
  editor.addEventListener("click", refresh);
  editor.addEventListener("keydown", (event: KeyboardEvent) => {
    if (menu.hidden) return;
    switch (event.key) {
      case "ArrowDown":
        if (matches.length) {
          event.preventDefault();
          active = (active + 1) % matches.length;
          render();
        }
        break;
      case "ArrowUp":
        if (matches.length) {
          event.preventDefault();
          active = (active - 1 + matches.length) % matches.length;
          render();
        }
        break;
      case "Enter":
        if (matches.length) {
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

    const option = target.closest<HTMLElement>("[data-command-index]");

    if (option) choose(Number(option.dataset.commandIndex));
  });
}

// Initializes slash commands.
export function initSlashCommands(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupSlashCommands(form);
}
