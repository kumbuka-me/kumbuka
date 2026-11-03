// Markdown toolbar selection and insertion helpers.

import { formatMarkdownDocument } from "./formatter.ts";
import { dispatchEditorEvent } from "./events.ts";
import { openCodeLanguagePicker } from "./code-language-picker.ts";
import {
  headingNumberFormat,
  renumberMarkdownHeadings,
} from "./heading-numbering.ts";

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

// Closes open toolbar menus except the one containing the click target.
function closeToolbarMenus(toolbar: HTMLElement, target?: Node): void {
  for (const menu of toolbar.querySelectorAll(".markdown-toolbar-menu[open]")) {
    if (!target || !menu.contains(target)) menu.removeAttribute("open");
  }
}

// Reveals icon labels while Alt is held and reliably clears them if the window
// loses focus before the matching keyup event arrives.
function setupToolbarIconNameReveal(toolbar: HTMLElement): void {
  function setVisible(visible: boolean): void {
    if (visible) toolbar.dataset.showIconNames = "true";
    else delete toolbar.dataset.showIconNames;
  }

  window.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key === "Alt") setVisible(true);
  });
  window.addEventListener("keyup", (event: KeyboardEvent) => {
    if (event.key === "Alt") setVisible(false);
  });
  window.addEventListener("blur", () => setVisible(false));
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) setVisible(false);
  });
}

// Wires markdown toolbar behavior.
function setupMarkdownToolbar(toolbar: HTMLElement): void {
  const form = toolbar.closest<HTMLFormElement>("[data-editor-form]");
  const textarea = form?.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  if (!textarea) return;

  const editor = textarea;
  setupToolbarIconNameReveal(toolbar);
  document.addEventListener(
    "click",
    (event) => {
      if (event.target instanceof Node)
        closeToolbarMenus(toolbar, event.target);
    },
    { capture: true },
  );

  let previousFormat = headingNumberFormat(form);
  function renumberIfEnabled(): void {
    const format = headingNumberFormat(form);
    if (!format) return;
    const numbered = renumberMarkdownHeadings(
      editor.value,
      format,
      previousFormat || format,
    );
    previousFormat = format;
    if (numbered === editor.value) return;
    const start = editor.selectionStart ?? 0;
    const end = editor.selectionEnd ?? start;
    editor.value = numbered;
    editor.setSelectionRange(
      Math.min(start, numbered.length),
      Math.min(end, numbered.length),
    );
  }

  function syncHeadingLabel(): void {
    if (form?.dataset.editorMode === "visual") return;

    const headingMenu = toolbar.querySelector<HTMLElement>(
      ".markdown-toolbar-heading-menu",
    );
    const label = headingMenu?.querySelector<HTMLElement>(
      "[data-heading-label]",
    );
    const summary = headingMenu?.querySelector<HTMLElement>("summary");
    if (!headingMenu || !label || !summary) return;

    const selection = editor.selectionStart ?? 0;
    const lineStart =
      editor.value.lastIndexOf("\n", Math.max(0, selection - 1)) + 1;
    const lineEnd = editor.value.indexOf("\n", selection);
    const line = editor.value.slice(
      lineStart,
      lineEnd < 0 ? undefined : lineEnd,
    );
    const level = /^(#{1,6})\s+/.exec(line)?.[1].length;
    const headingLabel = level ? `H${level}` : "Heading";

    label.textContent = headingLabel;
    summary.title = headingLabel;
    summary.setAttribute("aria-label", headingLabel);
    for (const button of headingMenu.querySelectorAll<HTMLButtonElement>(
      "[data-markdown-action^=heading-]",
    )) {
      const active = button.dataset.markdownAction === `heading-${level}`;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    }
  }

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

  // insertCodeBlock chooses a fenced-code language before creating or updating a block.
  async function insertCodeBlock(): Promise<void> {
    const language = await openCodeLanguagePicker("");
    if (language === null) return;
    if (visualCommand("code-block", { language })) return;

    const content = selectedText("command");
    const opening = `\`\`\`${language}\n`;
    const replacement = `${opening}${content}\n\`\`\``;
    replaceMarkdownSelection(
      editor,
      replacement,
      opening.length,
      opening.length + content.length,
    );
  }

  // Applies action.
  function applyAction(action: string | undefined): void {
    if (!action) return;
    if (action === "code-block") {
      void insertCodeBlock();
      return;
    }
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
      case "bullet-list":
        prefixMarkdownLines(editor, () => "- ", "item");
        break;
      case "ordered-list":
        prefixMarkdownLines(editor, (index) => `${index + 1}. `, "item");
        break;
      case "horizontal-rule":
        insertMarkdownAtSelection(editor, "---");
        break;
      case "clear-formatting": {
        const start = editor.selectionStart ?? 0;
        const end = editor.selectionEnd ?? start;
        const lineStart =
          editor.value.lastIndexOf("\n", Math.max(0, start - 1)) + 1;
        const lineEnd = editor.value.indexOf("\n", end);
        const replacement = editor.value
          .slice(lineStart, lineEnd < 0 ? undefined : lineEnd)
          .replace(/^#{1,6}\s+/, "")
          .replace(/^>\s?/, "");
        const originalEnd = lineEnd < 0 ? editor.value.length : lineEnd;
        editor.setRangeText(replacement, lineStart, originalEnd, "select");
        editor.focus();
        editor.dispatchEvent(new Event("input", { bubbles: true }));
        break;
      }
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
        markdown: pluginInsert.dataset.pluginInsertMarkdown ?? "",
        suffix: pluginInsert.dataset.pluginInsertSuffix,
        placeholder: pluginInsert.dataset.pluginInsertPlaceholder,
        mode: pluginInsert.dataset.pluginInsertMode,
        inline: pluginInsert.dataset.pluginInsertInline === "true",
      };
      if (!visualCommand("plugin-insert", { insert }))
        applyEditorInsertAction(editor, insert);
      closeToolbarMenus(toolbar);
      return;
    }

    const button = target.closest<HTMLElement>("[data-markdown-action]");
    if (!button) return;

    applyAction(button.dataset.markdownAction);
    syncHeadingLabel();
    closeToolbarMenus(toolbar);
  });

  editor.addEventListener("input", () => {
    renumberIfEnabled();
    syncHeadingLabel();
  });
  editor.addEventListener("click", syncHeadingLabel);
  editor.addEventListener("keyup", syncHeadingLabel);
  editor.addEventListener("select", syncHeadingLabel);
  syncHeadingLabel();
  form
    ?.querySelector<HTMLSelectElement>("[data-heading-numbering]")
    ?.addEventListener("change", () => {
      renumberIfEnabled();
      editor.dispatchEvent(new Event("input", { bubbles: true }));
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
