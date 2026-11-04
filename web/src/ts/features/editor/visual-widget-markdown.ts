// Independent Markdown editors for plugin widget fields and table cells.
// Each editor owns its own selection and undo history; the containing page
// editor only receives serialized Markdown when the widget form is applied.

import { Editor } from "./visual-deps/core.ts";
import { Markdown } from "./visual-deps/markdown.ts";
import { StarterKit } from "./visual-deps/starter.ts";
import { visualTaskListExtensions } from "./visual-deps/list.ts";
import { visualCodeLanguages } from "./visual-code-languages.ts";

interface MarkdownField {
  editor: Editor;
  source: HTMLTextAreaElement;
  resetDocument: () => void;
}

const fields = new WeakMap<HTMLTextAreaElement, MarkdownField>();

// createMarkdownControl displays Markdown as editable rich content with source-mode fallback.
export function createMarkdownControl(
  value: string,
  label: string,
  placeholder: string,
): { element: HTMLElement; source: HTMLTextAreaElement } {
  const field = document.createElement("div");
  field.className = "visual-widget-markdown-field";

  const modes = document.createElement("div");
  modes.className = "visual-widget-markdown-modes";
  const visual = document.createElement("button");
  visual.type = "button";
  visual.textContent = "Visual";
  visual.setAttribute("aria-pressed", "true");
  const markdown = document.createElement("button");
  markdown.type = "button";
  markdown.textContent = "Markdown";
  markdown.setAttribute("aria-pressed", "false");
  modes.append(visual, markdown);

  const toolbar = document.createElement("div");
  toolbar.className = "visual-widget-markdown-toolbar";
  const surface = document.createElement("div");
  surface.className = "visual-widget-markdown-surface";
  const source = document.createElement("textarea");
  source.value = value;
  source.rows = 6;
  source.placeholder = placeholder;
  source.setAttribute("aria-label", `${label} Markdown source`);
  source.hidden = true;
  field.append(modes, toolbar, surface, source);

  // Only user edits may replace the original Markdown. Tiptap can reserialize
  // equivalent content while initializing or switching between editor modes.
  let userEditPending = false;
  let lastDocument = "";
  const markUserEdit = () => {
    userEditPending = true;
  };
  surface.addEventListener("keydown", (event) => {
    if (
      event.key.length === 1 ||
      ["Backspace", "Delete", "Enter", "Tab"].includes(event.key) ||
      event.ctrlKey ||
      event.metaKey
    )
      markUserEdit();
  });
  surface.addEventListener("beforeinput", markUserEdit);
  surface.addEventListener("paste", markUserEdit);
  surface.addEventListener("drop", markUserEdit);
  surface.addEventListener("cut", markUserEdit);
  surface.addEventListener("click", (event) => {
    if (
      event.target instanceof Element &&
      event.target.closest(".visual-code-language")
    )
      markUserEdit();
  });

  const editor = new Editor({
    element: surface,
    extensions: [
      StarterKit.configure({ link: { openOnClick: false }, listItem: false }),
      ...visualTaskListExtensions(),
      visualCodeLanguages(),
      Markdown.configure({ markedOptions: { gfm: true, breaks: false } }),
    ],
    content: value,
    contentType: "markdown",
    editorProps: {
      attributes: {
        class: "prose visual-editor-content visual-widget-markdown-content",
        role: "textbox",
        "aria-label": label,
        "aria-multiline": "true",
        spellcheck: "true",
      },
    },
    onUpdate: ({ editor: current }) => {
      const currentDocument = JSON.stringify(current.getJSON());
      if (currentDocument === lastDocument) return;
      lastDocument = currentDocument;
      if (!userEditPending) return;
      userEditPending = false;
      source.value = current.getMarkdown();
      source.dispatchEvent(new Event("input", { bubbles: true }));
    },
  });
  lastDocument = JSON.stringify(editor.getJSON());
  fields.set(source, {
    editor,
    source,
    resetDocument: () => {
      lastDocument = JSON.stringify(editor.getJSON());
      userEditPending = false;
    },
  });

  const commands: Array<{ label: string; run: () => void }> = [
    { label: "Bold", run: () => editor.chain().focus().toggleBold().run() },
    { label: "Italic", run: () => editor.chain().focus().toggleItalic().run() },
    {
      label: "Bullet list",
      run: () => editor.chain().focus().toggleBulletList().run(),
    },
    {
      label: "Numbered list",
      run: () => editor.chain().focus().toggleOrderedList().run(),
    },
    {
      label: "Quote",
      run: () => editor.chain().focus().toggleBlockquote().run(),
    },
    {
      label: "Code block",
      run: () => editor.chain().focus().toggleCodeBlock().run(),
    },
  ];
  for (const command of commands) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = command.label;
    button.addEventListener("mousedown", (event) => event.preventDefault());
    button.addEventListener("click", () => {
      markUserEdit();
      command.run();
    });
    toolbar.append(button);
  }

  const showSource = (enabled: boolean) => {
    if (enabled === !source.hidden) return;
    if (enabled) {
      // Preserve original bytes until a real edit occurs.
      surface.hidden = true;
      toolbar.hidden = true;
      source.hidden = false;
      source.focus();
    } else {
      editor.commands.setContent(source.value, {
        contentType: "markdown",
        emitUpdate: false,
      });
      lastDocument = JSON.stringify(editor.getJSON());
      userEditPending = false;
      source.hidden = true;
      surface.hidden = false;
      toolbar.hidden = false;
      editor.commands.focus();
    }
    visual.setAttribute("aria-pressed", String(!enabled));
    markdown.setAttribute("aria-pressed", String(enabled));
  };
  visual.addEventListener("click", () => showSource(false));
  markdown.addEventListener("click", () => showSource(true));
  return { element: field, source };
}

// setMarkdownControlValue synchronizes programmatic resets with their visual editors.
export function setMarkdownControlValue(
  source: HTMLTextAreaElement,
  value: string,
): void {
  source.value = value;
  const field = fields.get(source);
  field?.editor.commands.setContent(value, {
    contentType: "markdown",
    emitUpdate: false,
  });
  field?.resetDocument();
}

// disposeMarkdownControls releases nested editors before their dialog or row is removed.
export function disposeMarkdownControls(container: HTMLElement): void {
  for (const source of container.querySelectorAll<HTMLTextAreaElement>(
    ".visual-widget-markdown-field textarea",
  )) {
    fields.get(source)?.editor.destroy();
    fields.delete(source);
  }
}
