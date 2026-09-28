import { route } from "../../core/route.ts";
import { visualPane } from "./visual-pane.ts";
import { loadEditorCatalog, type CatalogCompletion } from "./catalog.ts";
import { openCompletionPicker } from "./completion-picker.ts";
// Confluence-style visual editing backed by the canonical Markdown textarea.

import { Editor } from "./visual-deps/core.ts";
import { Image } from "./visual-deps/image.ts";
import { TableKit } from "./visual-deps/table.ts";
import { Markdown } from "./visual-deps/markdown.ts";
import { StarterKit } from "./visual-deps/starter.ts";

import type { EditorInsertAction } from "./toolbar.ts";
import { markdownTables, rewriteTableDirectiveSource } from "./tables.ts";
import type { CatalogWidget } from "./widget-contract.ts";
import { visualWidgetNodes } from "./visual-widget-node.ts";
import { visualCodeLanguages } from "./visual-code-languages.ts";
import {
  currentVisualTableContext,
  kumbukaBlockNode,
  kumbukaInlineNode,
  pluginMarkdown,
  slashCommands,
  visualMentions,
  visualTableStyles,
  type VisualEditor,
} from "./visual-extensions.ts";
import {
  mentionReplacement,
  mentionTrigger,
  renderMentionSuggestions,
  searchMentionUsers,
  type MentionUser,
} from "../mentions.ts";

export {
  matchKumbukaBlockSyntax,
  matchKumbukaInlineSyntax,
  visualSyntaxLabel,
} from "./visual-syntax.ts";


export function setupVisualEditor(form: HTMLFormElement): void {
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const workspace = form.querySelector<HTMLElement>("[data-editor-workspace]");
  const visualButton = form.querySelector<HTMLButtonElement>(
    'button[data-editor-mode="visual"]',
  );
  const sharedToolbar = form.querySelector<HTMLElement>(
    "[data-markdown-toolbar]",
  );
  if (!source || !workspace || !visualButton || !sharedToolbar) return;

  const toolbar = sharedToolbar;
  const pane = visualPane(workspace);
  const surface = pane.querySelector<HTMLElement>("[data-visual-editor]");
  const status = pane.querySelector<HTMLElement>("[data-visual-status]");
  const slashMenu = pane.querySelector<HTMLElement>("[data-visual-slash-menu]");
  if (!surface || !status || !slashMenu) return;

  const markdownSource = source;
  const visualSurface = surface;
  const visualStatus = status;
  const visualSlashMenu = slashMenu;
  const mentionMenu = document.createElement("div");

  mentionMenu.className = "mention-suggestion-menu visual-mention-menu";
  mentionMenu.id = "visual-mention-suggestions";
  mentionMenu.hidden = true;
  mentionMenu.setAttribute("role", "listbox");
  mentionMenu.setAttribute("aria-label", "Mention a user");
  pane.append(mentionMenu);

  let editor: VisualEditor | null = null;
  let loading: Promise<void> | null = null;
  let syncingFromVisual = false;
  let slashFrom = -1;
  let slashSelection = 0;
  let mentionFrom = -1;
  let mentionQuery = "";
  let mentionResults: MentionUser[] = [];
  let mentionSelection = 0;
  let mentionRequest = 0;
  const commands = slashCommands();
  let widgetContracts: CatalogWidget[] | null = null;
  let widgetCompletions: CatalogCompletion[] = [];
  let widgetContractProblem = "";

  async function loadWidgetContracts(): Promise<CatalogWidget[]> {
    if (widgetContracts) return widgetContracts;
    try {
      const catalog = await loadEditorCatalog();
      widgetContracts = catalog.widgets;
      widgetCompletions = catalog.completions;
      if (catalog.widget_problems.length)
        widgetContractProblem = catalog.widget_problems
          .map((problem) => `${problem.plugin_id}: ${problem.message}`)
          .join("; ");
    } catch (error) {
      console.error("visual widget contracts could not be loaded", error);
      widgetContracts = [];
      widgetContractProblem =
        "Plugin visual-editor widgets could not be loaded. Source editing remains available.";
    }
    return widgetContracts;
  }

  function visualMode(): boolean {
    return form.dataset.editorMode === "visual";
  }

  function hideSlashMenu(): void {
    visualSlashMenu.hidden = true;
    visualSlashMenu.replaceChildren();
    slashFrom = -1;
    slashSelection = 0;
  }

  function hideMentionMenu(): void {
    mentionRequest += 1;
    mentionFrom = -1;
    mentionQuery = "";
    mentionResults = [];
    mentionSelection = 0;
    mentionMenu.hidden = true;
    mentionMenu.replaceChildren();
    editor?.view.dom.removeAttribute("aria-controls");
    editor?.view.dom.setAttribute("aria-expanded", "false");
  }

  function renderVisualMentions(): void {
    if (!editor || mentionFrom < 0) return;
    renderMentionSuggestions(
      mentionMenu,
      mentionResults,
      mentionQuery,
      mentionSelection,
    );
    mentionMenu.hidden = false;
    editor.view.dom.setAttribute("aria-controls", mentionMenu.id);
    editor.view.dom.setAttribute("aria-expanded", "true");
    const coords = editor.view.coordsAtPos(editor.state.selection.from);
    const width = Math.min(390, Math.max(180, window.innerWidth - 16));
    const top =
      coords.bottom + 336 <= window.innerHeight
        ? coords.bottom + 6
        : Math.max(8, coords.top - 336);
    mentionMenu.style.width = `${width}px`;
    mentionMenu.style.left = `${Math.max(8, Math.min(coords.left, window.innerWidth - width - 8))}px`;
    mentionMenu.style.top = `${top}px`;
  }

  async function refreshMentionMenu(): Promise<void> {
    if (!editor || !visualMode()) {
      hideMentionMenu();
      return;
    }
    const selection = editor.state.selection;
    const $from = selection?.$from;
    if (
      !selection?.empty ||
      !$from?.parent ||
      editor.isActive("codeBlock") ||
      editor.isActive("code")
    ) {
      hideMentionMenu();
      return;
    }
    const before = String(
      $from.parent.textBetween(0, $from.parentOffset, "\n", "\n") || "",
    );
    const trigger = mentionTrigger(before, before.length);
    if (!trigger) {
      hideMentionMenu();
      return;
    }

    mentionFrom = selection.from - (before.length - trigger.start);
    mentionQuery = trigger.query;
    mentionSelection = 0;
    hideSlashMenu();
    const currentRequest = ++mentionRequest;
    try {
      const results = await searchMentionUsers(trigger.query);
      if (currentRequest !== mentionRequest) return;
      mentionResults = results;
      renderVisualMentions();
    } catch (error) {
      console.error("mention search failed", error);
      if (currentRequest === mentionRequest) hideMentionMenu();
    }
  }

  function chooseMention(index: number): void {
    const user = mentionResults[index];
    if (!editor || !user || mentionFrom < 0) return;
    const to = editor.state.selection.from;
    editor
      .chain()
      .focus()
      .deleteRange({ from: mentionFrom, to })
      .insertContent(mentionReplacement(user.username))
      .run();
    hideMentionMenu();
  }

  function moveMentionSelection(direction: number): void {
    if (!mentionResults.length) return;
    mentionSelection =
      (mentionSelection + direction + mentionResults.length) %
      mentionResults.length;
    renderVisualMentions();
    mentionMenu
      .querySelector<HTMLElement>('[aria-selected="true"]')
      ?.scrollIntoView({ block: "nearest" });
  }

  function clearToolbarState(): void {
    for (const button of toolbar.querySelectorAll<HTMLButtonElement>(
      "button[data-markdown-action]",
    )) {
      button.classList.remove("active");
      button.removeAttribute("aria-pressed");
    }
  }

  function syncTableContext(): void {
    if (!editor || !visualMode()) {
      form.dispatchEvent(
        new CustomEvent("editor:visual-table-context", { detail: null }),
      );
      return;
    }

    form.dispatchEvent(
      new CustomEvent("editor:visual-table-context", {
        detail: currentVisualTableContext(editor),
      }),
    );
  }

  function syncToolbar(): void {
    if (!editor || !visualMode()) {
      clearToolbarState();
      syncTableContext();
      return;
    }

    const active: Record<string, boolean> = {
      quote: editor.isActive("blockquote"),
      "code-block": editor.isActive("codeBlock"),
      ...Object.fromEntries(
        [1, 2, 3, 4, 5, 6].map((level) => [
          `heading-${level}`,
          editor!.isActive("heading", { level }),
        ]),
      ),
      bold: editor.isActive("bold"),
      italic: editor.isActive("italic"),
      underline: editor.isActive("underline"),
      strike: editor.isActive("strike"),
      "inline-code": editor.isActive("code"),
      link: editor.isActive("link"),
      "bullet-list": editor.isActive("bulletList"),
      "ordered-list": editor.isActive("orderedList"),
    };

    for (const button of toolbar.querySelectorAll<HTMLButtonElement>(
      "button[data-markdown-action]",
    )) {
      const action = button.dataset.markdownAction || "";
      if (!(action in active)) {
        button.classList.remove("active");
        button.removeAttribute("aria-pressed");
        continue;
      }

      const selected = active[action] || false;
      button.classList.toggle("active", selected);
      button.setAttribute("aria-pressed", String(selected));
    }

    syncTableContext();
  }

  function restoreTableDimensions(): void {
    if (!editor) return;
    const tables = markdownTables(markdownSource.value);
    const transaction = editor.state.tr;
    let index = 0;
    editor.state.doc.descendants((table: any, position: number) => {
      if (table.type.name !== "table") return true;
      const directive = tables[index++]?.directive;
      table.forEach((row: any, offset: number, rowIndex: number) => {
        const rowPosition = position + 1 + offset;
        if (directive?.heights?.[rowIndex])
          transaction.setNodeMarkup(rowPosition, undefined, {
            ...row.attrs,
            height: directive.heights[rowIndex],
          });
        row.forEach((cell: any, cellOffset: number, column: number) => {
          if (directive?.widths?.[column])
            transaction.setNodeMarkup(rowPosition + 1 + cellOffset, undefined, {
              ...cell.attrs,
              colwidth: [directive.widths[column]],
            });
        });
      });
      return false;
    });
    if (transaction.docChanged)
      editor.view.dispatch(
        transaction
          .setMeta("preventUpdate", true)
          .setMeta("addToHistory", false),
      );
  }

  function reloadVisualContent(preserveSelection = true): void {
    if (!editor) return;

    const previous = editor.state?.selection;
    editor.commands.setContent(markdownSource.value, {
      contentType: "markdown",
      emitUpdate: false,
    });

    restoreTableDimensions();
    if (preserveSelection && previous) {
      const limit = Math.max(1, Number(editor.state?.doc?.content?.size || 1));
      const from = Math.max(1, Math.min(Number(previous.from) || 1, limit));
      const to = Math.max(from, Math.min(Number(previous.to) || from, limit));
      editor.chain().setTextSelection({ from, to }).run();
    }
  }

  function syncMarkdown(): void {
    if (!editor) return;
    let markdown = editor.getMarkdown();
    let tableIndex = 0;
    editor.state.doc.descendants((table: any) => {
      if (table.type.name !== "table") return true;
      const parsed = markdownTables(markdown)[tableIndex++];
      if (!parsed) return false;
      const widths: number[] = [];
      const heights: number[] = [];
      table.firstChild?.forEach((cell: any) =>
        widths.push(Number(cell.attrs.colwidth?.[0]) || 0),
      );
      table.forEach((row: any) => heights.push(Number(row.attrs.height) || 0));
      const directive = { ...parsed.directive };
      if (widths.some(Boolean)) {
        const cells =
          visualSurface.querySelectorAll("table")[tableIndex - 1]?.rows[0]
            ?.cells;
        directive.widths = widths.map(
          (width, index) =>
            width ||
            Math.max(
              40,
              Math.round(cells?.[index]?.getBoundingClientRect().width || 100),
            ),
        );
      } else delete directive.widths;
      if (heights.some(Boolean)) directive.heights = heights;
      else delete directive.heights;
      markdown = rewriteTableDirectiveSource(markdown, parsed, directive);
      return false;
    });
    if (markdown === markdownSource.value) {
      return;
    }

    syncingFromVisual = true;
    markdownSource.value = markdown;
    markdownSource.dispatchEvent(new Event("input", { bubbles: true }));
    syncingFromVisual = false;
  }

  function refreshSlashMenu(): void {
    if (!editor || !visualMode()) {
      hideSlashMenu();
      return;
    }
    const selection = editor.state?.selection;
    const from = selection?.from;
    const $from = selection?.$from;
    if (!selection?.empty || typeof from !== "number" || !$from?.parent) {
      hideSlashMenu();
      return;
    }

    const before = String(
      $from.parent.textBetween(0, $from.parentOffset, "\n", "\n") || "",
    );
    const match = /(?:^|\s)\/([A-Za-z0-9-]*)$/.exec(before);
    if (!match) {
      hideSlashMenu();
      return;
    }

    const query = (match[1] || "").toLocaleLowerCase();
    const matches = commands.filter((command) =>
      (command.label + " " + command.keywords)
        .toLocaleLowerCase()
        .includes(query),
    );
    if (!matches.length) {
      hideSlashMenu();
      return;
    }

    slashFrom = from - query.length - 1;
    slashSelection = Math.min(slashSelection, matches.length - 1);
    const fragment = document.createDocumentFragment();

    matches.forEach((command, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.dataset.visualSlashCommand = command.id;
      button.setAttribute("role", "option");
      button.setAttribute("aria-selected", String(index === slashSelection));
      button.innerHTML = "<strong></strong><small>/" + command.id + "</small>";
      const strong = button.querySelector("strong");
      if (strong) strong.textContent = command.label;
      button.addEventListener("mousedown", (event) => {
        event.preventDefault();
        command.run(editor as VisualEditor, slashFrom, from);
        hideSlashMenu();
      });
      fragment.append(button);
    });

    visualSlashMenu.replaceChildren(fragment);
    visualSlashMenu.hidden = false;
    try {
      const coords = editor.view.coordsAtPos(from);
      visualSlashMenu.style.left = Math.max(8, coords.left) + "px";
      visualSlashMenu.style.top = Math.max(8, coords.bottom + 6) + "px";
    } catch {
      hideSlashMenu();
    }
  }

  function moveSlashSelection(direction: number): void {
    const items = [
      ...visualSlashMenu.querySelectorAll<HTMLButtonElement>(
        "button[data-visual-slash-command]",
      ),
    ];
    if (!items.length) return;
    slashSelection = (slashSelection + direction + items.length) % items.length;
    items.forEach((item, index) =>
      item.setAttribute("aria-selected", String(index === slashSelection)),
    );
    items[slashSelection]?.scrollIntoView({ block: "nearest" });
  }

  function setBlockStyle(style: string): void {
    if (!editor) return;
    const chain = editor.chain().focus();
    switch (style) {
      case "heading-1":
        chain.setHeading({ level: 1 }).run();
        break;
      case "heading-2":
        chain.setHeading({ level: 2 }).run();
        break;
      case "heading-3":
        chain.setHeading({ level: 3 }).run();
        break;
      case "blockquote":
        chain.setBlockquote().run();
        break;
      case "code-block":
        chain.setCodeBlock().run();
        break;
      default:
        chain.setParagraph().run();
    }
    syncToolbar();
  }

  // runPluginInsert opens a resource picker when available, then inserts the selected Markdown.
  async function runPluginInsert(insert: EditorInsertAction): Promise<void> {
    if (!editor) return;

    const opened = await openCompletionPicker(insert, (replacement) => {
      if (!editor) return;
      editor
        .chain()
        .focus()
        .insertContent(replacement, { contentType: "markdown" })
        .run();
      syncToolbar();
    });
    if (opened || !editor) return;

    const markdown = pluginMarkdown(insert, editor);
    if (markdown)
      editor
        .chain()
        .focus()
        .insertContent(markdown, { contentType: "markdown" })
        .run();
    syncToolbar();
  }

  function runToolbarCommand(
    action: string,
    detail: Record<string, unknown>,
  ): void {
    if (!editor || !visualMode()) return;
    const chain = editor.chain().focus();
    if (/^heading-[1-6]$/.test(action)) {
      chain.toggleHeading({ level: Number(action.slice(-1)) }).run();
      syncToolbar();
      return;
    }

    switch (action) {
      case "quote":
        chain.toggleBlockquote().run();
        break;
      case "code-block":
        chain
          .setCodeBlock({ language: String(detail.language || "") || null })
          .run();
        break;
      case "bold":
        chain.toggleBold().run();
        break;
      case "italic":
        chain.toggleItalic().run();
        break;
      case "underline":
        chain.toggleUnderline().run();
        break;
      case "strike":
        chain.toggleStrike().run();
        break;
      case "inline-code":
        chain.toggleCode().run();
        break;
      case "mention":
        chain.insertContent("@").run();
        break;
      case "link": {
        const current = String(editor.getAttributes("link").href || "");
        const href = window.prompt("Link URL", current || "https://");
        if (href === null) break;
        if (!href.trim()) chain.unsetLink().run();
        else if (editor.state.selection.empty && !editor.isActive("link"))
          chain
            .insertContent({
              type: "text",
              text: href.trim(),
              marks: [{ type: "link", attrs: { href: href.trim() } }],
            })
            .run();
        else chain.extendMarkRange("link").setLink({ href: href.trim() }).run();
        break;
      }
      case "bullet-list":
        chain.toggleBulletList().run();
        break;
      case "ordered-list":
        chain.toggleOrderedList().run();
        break;
      case "horizontal-rule":
        chain.setHorizontalRule().run();
        break;
      case "undo":
        chain.undo().run();
        break;
      case "redo":
        chain.redo().run();
        break;
      case "block-style":
        setBlockStyle(String(detail.style || "paragraph"));
        return;
      case "plugin-insert": {
        const insert = detail.insert as EditorInsertAction | undefined;
        if (!insert?.markdown) return;
        void runPluginInsert(insert);
        return;
      }
      default:
        return;
    }

    syncToolbar();
  }

  async function ensureEditor(): Promise<void> {
    if (editor) return;
    if (loading) return loading;

    visualStatus.hidden = false;
    visualStatus.classList.remove("error");
    visualStatus.textContent = "Loading visual editor…";

    loading = (async () => {
      try {
        const widgets = await loadWidgetContracts();
        editor = new Editor({
          element: visualSurface,
          extensions: [
            StarterKit.configure({ link: { openOnClick: false } }),
            visualMentions(),
            TableKit.configure({
              table: { resizable: true, cellMinWidth: 40 },
            }),
            Image.extend({
              renderHTML({ HTMLAttributes }) {
                const attributes = {
                  ...this.options.HTMLAttributes,
                  ...HTMLAttributes,
                };
                if (typeof attributes.src === "string")
                  attributes.src = route(attributes.src);
                return ["img", attributes];
              },
            }).configure({ allowBase64: false }),
            visualCodeLanguages(),
            visualTableStyles(() => markdownSource.value),
            ...visualWidgetNodes(widgets, widgetCompletions),
            kumbukaInlineNode(widgets),
            kumbukaBlockNode(),
            Markdown.configure({ markedOptions: { gfm: true, breaks: false } }),
          ],
          content: markdownSource.value,
          contentType: "markdown",
          editorProps: {
            attributes: {
              class: "prose visual-editor-content",
              spellcheck: "true",
            },
          },
          onUpdate: () => {
            syncMarkdown();
            syncToolbar();
            refreshSlashMenu();
            void refreshMentionMenu();
          },
          onSelectionUpdate: () => {
            syncToolbar();
            refreshSlashMenu();
            void refreshMentionMenu();
          },
        });
        restoreTableDimensions();
        if (widgetContractProblem) {
          visualStatus.hidden = false;
          visualStatus.classList.add("error");
          visualStatus.textContent = widgetContractProblem;
        } else {
          visualStatus.hidden = true;
        }
        syncToolbar();
      } catch (error) {
        console.error("visual editor failed to load", error);
        visualStatus.hidden = false;
        visualStatus.classList.add("error");
        visualStatus.textContent =
          "Visual editor could not be loaded. Markdown mode remains available.";
        throw error;
      } finally {
        loading = null;
      }
    })();

    return loading;
  }

  visualSurface.addEventListener(
    "keydown",
    (event) => {
      if (!mentionMenu.hidden) {
        switch (event.key) {
          case "ArrowDown":
            event.preventDefault();
            event.stopPropagation();
            moveMentionSelection(1);
            return;
          case "ArrowUp":
            event.preventDefault();
            event.stopPropagation();
            moveMentionSelection(-1);
            return;
          case "Enter":
          case "Tab":
            if (mentionResults.length) {
              event.preventDefault();
              event.stopPropagation();
              chooseMention(mentionSelection);
            }
            return;
          case "Escape":
            event.preventDefault();
            event.stopPropagation();
            hideMentionMenu();
            return;
        }
      }
      if (visualSlashMenu.hidden) return;
      switch (event.key) {
        case "ArrowDown":
          event.preventDefault();
          event.stopPropagation();
          moveSlashSelection(1);
          break;
        case "ArrowUp":
          event.preventDefault();
          event.stopPropagation();
          moveSlashSelection(-1);
          break;
        case "Enter": {
          const selected = visualSlashMenu.querySelector<HTMLButtonElement>(
            'button[aria-selected="true"]',
          );
          if (!selected) return;
          event.preventDefault();
          event.stopPropagation();
          selected.dispatchEvent(
            new MouseEvent("mousedown", { bubbles: true }),
          );
          break;
        }
        case "Escape":
          event.preventDefault();
          event.stopPropagation();
          hideSlashMenu();
          break;
      }
    },
    true,
  );

  mentionMenu.addEventListener("mousedown", (event) => event.preventDefault());
  mentionMenu.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;
    const option = target.closest<HTMLElement>("[data-mention-index]");
    if (option) chooseMention(Number(option.dataset.mentionIndex));
  });

  form.addEventListener("editor:visual-command", (event) => {
    if (!(event instanceof CustomEvent)) return;
    const detail =
      typeof event.detail === "object" && event.detail !== null
        ? (event.detail as Record<string, unknown>)
        : {};
    runToolbarCommand(String(detail.action || ""), detail);
  });

  form.addEventListener("editor:visual-table-insert", (event) => {
    if (!(event instanceof CustomEvent) || !editor || !visualMode()) return;
    const detail = event.detail as { rows?: number; columns?: number } | null;
    const rows = Math.max(1, Math.min(10, Number(detail?.rows) || 3));
    const cols = Math.max(1, Math.min(10, Number(detail?.columns) || 3));
    editor
      .chain()
      .focus()
      .insertTable({ rows: rows + 1, cols, withHeaderRow: true })
      .run();
    syncToolbar();
  });

  markdownSource.addEventListener("input", () => {
    if (!editor || syncingFromVisual) return;
    reloadVisualContent();
    syncToolbar();
  });

  form.addEventListener("editor:restore-draft", () => {
    if (!editor) return;
    requestAnimationFrame(() => {
      reloadVisualContent(false);
      syncToolbar();
    });
  });

  form.addEventListener("editor:visual-activate", () => {
    void ensureEditor()
      .then(() => {
        if (!editor || !visualMode()) return;
        editor.chain().focus().run();
        syncToolbar();
      })
      .catch(() => undefined);
  });

  form.addEventListener("editor:mode-change", () => {
    if (visualMode()) syncToolbar();
    else {
      hideSlashMenu();
      hideMentionMenu();
      clearToolbarState();
      syncTableContext();
    }
  });

  window.addEventListener("pagehide", () => editor?.destroy(), { once: true });
}

// Initializes the visual editor beside the canonical Markdown source.
export function initVisualEditor(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupVisualEditor(form);
}
