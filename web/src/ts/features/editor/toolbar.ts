// Markdown toolbar selection and insertion helpers.

import { formatMarkdownDocument } from "./formatter.ts";
import { dispatchEditorEvent } from "./events.ts";

function insertIntoVisualEditor(
  textarea: HTMLTextAreaElement,
  markdown: string,
): boolean {
  const form = textarea.closest<HTMLFormElement>("[data-editor-form]");
  if (form?.dataset.editorMode !== "visual") return false;
  form.dispatchEvent(
    new CustomEvent("editor:visual-command", {
      detail: { action: "plugin-insert", insert: { markdown } },
    }),
  );
  return true;
}

// Inserts inline text at the textarea selection without adding line breaks.
export function insertInlineAtSelection(
  textarea: HTMLTextAreaElement,
  text: string,
): void {
  if (insertIntoVisualEditor(textarea, text)) return;
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;

  textarea.setRangeText(text, start, end, "end");
  textarea.focus();
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

// Inserts block Markdown at the textarea selection.
export function insertMarkdownAtSelection(
  textarea: HTMLTextAreaElement,
  markdown: string,
): void {
  if (insertIntoVisualEditor(textarea, markdown)) return;
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;
  const before = textarea.value.slice(0, start);
  const after = textarea.value.slice(end);
  const prefix = before && !before.endsWith("\n") ? "\n" : "";
  const suffix = after && !after.startsWith("\n") ? "\n" : "";
  const insertion = `${prefix}${markdown}${suffix}`;

  textarea.setRangeText(insertion, start, end, "end");
  textarea.focus();
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

// Replaces the selected Markdown and restores the selection.
function replaceMarkdownSelection(
  textarea: HTMLTextAreaElement,
  replacement: string,
  selectionStart = 0,
  selectionEnd = replacement.length,
): void {
  const start = textarea.selectionStart ?? textarea.value.length;
  const end = textarea.selectionEnd ?? start;

  textarea.setRangeText(replacement, start, end, "select");
  textarea.setSelectionRange(start + selectionStart, start + selectionEnd);
  textarea.focus();
  textarea.dispatchEvent(new Event("input", { bubbles: true }));
}

// Wraps the selected Markdown with delimiters.
function wrapMarkdownSelection(
  textarea: HTMLTextAreaElement,
  before: string,
  after: string,
  placeholder: string,
): void {
  const selected = textarea.value.slice(
    textarea.selectionStart ?? 0,
    textarea.selectionEnd ?? 0,
  );
  const content = selected || placeholder;
  const replacement = `${before}${content}${after}`;

  replaceMarkdownSelection(
    textarea,
    replacement,
    before.length,
    before.length + content.length,
  );
}

// Prefixes each selected Markdown line.
function prefixMarkdownLines(
  textarea: HTMLTextAreaElement,
  prefixer: (index: number) => string,
  placeholder = "item",
): void {
  const selected =
    textarea.value.slice(
      textarea.selectionStart ?? 0,
      textarea.selectionEnd ?? 0,
    ) || placeholder;
  const lines = selected.split("\n");
  const replacement = lines
    .map((line, index) => `${prefixer(index)}${line}`)
    .join("\n");

  replaceMarkdownSelection(textarea, replacement, 0, replacement.length);
}

export interface EditorInsertAction {
  pluginID?: string;
  name?: string;
  completionModuleID?: string;
  markdown: string;
  suffix?: string;
  placeholder?: string;
  mode?: string;
  inline?: boolean;
}

// Applies one host-validated declarative plugin editor action.
export function applyEditorInsertAction(
  editor: HTMLTextAreaElement,
  action: EditorInsertAction,
): void {
  if (!action.markdown) return;

  switch (action.mode || "insert") {
    case "wrap":
      wrapMarkdownSelection(
        editor,
        action.markdown,
        action.suffix ?? "",
        action.placeholder || "text",
      );
      break;
    case "prefix-lines":
      prefixMarkdownLines(
        editor,
        () => action.markdown,
        action.placeholder || "item",
      );
      break;
    default:
      if (action.inline) insertInlineAtSelection(editor, action.markdown);
      else insertMarkdownAtSelection(editor, action.markdown);
      break;
  }
}

// Closes open Markdown toolbar menus.
function closeToolbarMenus(toolbar: HTMLElement): void {
  for (const menu of toolbar.querySelectorAll(".markdown-toolbar-menu[open]")) {
    menu.removeAttribute("open");
  }
}

// Opens a resource-backed completion picker without coupling it to the visual editor bundle.
async function chooseVisualCompletion(
  insert: EditorInsertAction,
  onChoose: (replacement: string) => void,
): Promise<boolean> {
  try {
    const { openCompletionPicker } = await import("./completion-picker.ts");
    return await openCompletionPicker(insert, onChoose);
  } catch (error) {
    console.error("Could not open editor completion picker", error);
    return false;
  }
}

// Wires markdown toolbar behavior.
function setupMarkdownToolbar(toolbar: HTMLElement): void {
  const form = toolbar.closest<HTMLFormElement>("[data-editor-form]");
  const textarea = form?.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  if (!textarea) return;

  const editor = textarea;

  function visualCommand(
    action: string,
    detail: Record<string, unknown> = {},
  ): boolean {
    if (form?.dataset.editorMode !== "visual") return false;
    form.dispatchEvent(
      new CustomEvent("editor:visual-command", {
        detail: { action, ...detail },
      }),
    );
    return true;
  }

  // Returns selected Markdown or the action placeholder.
  function selectedText(placeholder: string): string {
    return (
      editor.value.slice(
        editor.selectionStart ?? 0,
        editor.selectionEnd ?? 0,
      ) || placeholder
    );
  }

  // Applies action.
  function applyAction(action: string | undefined): void {
    if (!action) return;
    if (
      action !== "find" &&
      action !== "format-document" &&
      visualCommand(action)
    )
      return;
    if (action.startsWith("heading-")) {
      const level = Number(action.slice("heading-".length));

      if (level >= 1 && level <= 6) {
        prefixMarkdownLines(editor, () => `${"#".repeat(level)} `, "Heading");
      }

      return;
    }

    switch (action) {
      case "mention":
        insertInlineAtSelection(editor, "@");
        break;
      case "bold":
        wrapMarkdownSelection(editor, "**", "**", "bold text");
        break;
      case "italic":
        wrapMarkdownSelection(editor, "*", "*", "italic text");
        break;
      case "link": {
        const label = selectedText("link text");
        const replacement = `[${label}](https://example.com)`;
        const urlStart = label.length + 3;

        replaceMarkdownSelection(
          editor,
          replacement,
          urlStart,
          urlStart + "https://example.com".length,
        );
        break;
      }
      case "quote":
        prefixMarkdownLines(editor, () => "> ", "Quoted text");
        break;
      case "inline-code":
        wrapMarkdownSelection(editor, "`", "`", "code");
        break;
      case "code-block": {
        const content = selectedText("command");
        const replacement = `\`\`\`\n${content}\n\`\`\``;

        replaceMarkdownSelection(editor, replacement, 4, 4 + content.length);
        break;
      }
      case "bullet-list":
        prefixMarkdownLines(editor, () => "- ", "item");
        break;
      case "ordered-list":
        prefixMarkdownLines(editor, (index) => `${index + 1}. `, "item");
        break;
      case "horizontal-rule":
        insertMarkdownAtSelection(editor, "---");
        break;
      case "find":
        if (form) dispatchEditorEvent(form, "editor:find");
        break;
      case "format-document": {
        const formatted = formatMarkdownDocument(editor.value);
        if (formatted === editor.value) {
          editor.focus();
          break;
        }

        const selectionStart = Math.min(
          editor.selectionStart ?? 0,
          formatted.length,
        );
        const selectionEnd = Math.min(
          editor.selectionEnd ?? selectionStart,
          formatted.length,
        );
        const scrollTop = editor.scrollTop;

        editor.value = formatted;
        editor.setSelectionRange(selectionStart, selectionEnd);
        editor.scrollTop = scrollTop;
        editor.focus();
        editor.dispatchEvent(new Event("input", { bubbles: true }));
        break;
      }
      default:
        break;
    }
  }

  toolbar.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const pluginInsert = target.closest<HTMLElement>(
      "[data-plugin-insert-markdown]",
    );
    if (pluginInsert) {
      const insert: EditorInsertAction = {
        pluginID: pluginInsert.dataset.pluginInsertPluginId,
        name: pluginInsert.dataset.pluginInsertName,
        completionModuleID: pluginInsert.dataset.pluginInsertCompletionModuleId,
        markdown: pluginInsert.dataset.pluginInsertMarkdown ?? "",
        suffix: pluginInsert.dataset.pluginInsertSuffix,
        placeholder: pluginInsert.dataset.pluginInsertPlaceholder,
        mode: pluginInsert.dataset.pluginInsertMode,
        inline: pluginInsert.dataset.pluginInsertInline === "true",
      };

      closeToolbarMenus(toolbar);
      if (form?.dataset.editorMode === "visual" && insert.completionModuleID) {
        void chooseVisualCompletion(insert, (replacement) => {
          visualCommand("plugin-insert", {
            insert: { ...insert, markdown: replacement },
          });
        }).then((opened) => {
          if (!opened) visualCommand("plugin-insert", { insert });
        });
        return;
      }

      if (!visualCommand("plugin-insert", { insert }))
        applyEditorInsertAction(editor, insert);
      return;
    }

    const button = target.closest<HTMLElement>("[data-markdown-action]");
    if (!button) return;

    applyAction(button.dataset.markdownAction);
    closeToolbarMenus(toolbar);
  });

  editor.addEventListener("keydown", (event: KeyboardEvent) => {
    if (!(event.ctrlKey || event.metaKey)) return;

    const key = event.key.toLocaleLowerCase();

    switch (key) {
      case "b":
        event.preventDefault();
        applyAction("bold");
        break;
      case "i":
        event.preventDefault();
        applyAction("italic");
        break;
      case "k":
        if (event.shiftKey) {
          event.preventDefault();
          applyAction("link");
        }
        break;
    }
  });
}

// Initializes markdown toolbar.
export function initMarkdownToolbar(): void {
  for (const toolbar of document.querySelectorAll<HTMLElement>(
    "[data-markdown-toolbar]",
  ))
    setupMarkdownToolbar(toolbar);
}
