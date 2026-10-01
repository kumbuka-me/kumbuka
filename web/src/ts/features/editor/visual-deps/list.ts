import {
  mergeAttributes,
  renderNestedMarkdownContent,
  type MarkdownParseHelpers,
  type MarkdownParseResult,
  type MarkdownToken,
} from "@tiptap/core";
import { ListItem } from "@tiptap/extension-list";

export interface VisualTaskListInsert {
  markdown: string;
  mode?: string;
}

interface VisualListEditor {
  chain(): any;
  isActive(name: string): boolean;
  state: any;
  view: any;
}

const baseParseMarkdown = ListItem.config.parseMarkdown;
const baseRenderMarkdown = ListItem.config.renderMarkdown;

function taskCheckboxLabel(node: any): string {
  return node?.attrs?.checked ? "Mark task incomplete" : "Mark task complete";
}

function applyDOMAttributes(
  element: HTMLElement,
  ...attributeSets: Array<Record<string, unknown> | undefined>
): void {
  for (const attributes of attributeSets) {
    if (!attributes) continue;
    for (const [name, value] of Object.entries(attributes)) {
      if (value === null || value === undefined || value === false) continue;
      if (name === "class") {
        for (const className of String(value).split(/\s+/).filter(Boolean))
          element.classList.add(className);
        continue;
      }
      element.setAttribute(name, value === true ? "" : String(value));
    }
  }
}

function checkedFromHTML(element: HTMLElement): boolean {
  const value = element.getAttribute("data-checked");
  if (value !== null) return value === "" || value === "true";
  return Boolean(
    element.querySelector<HTMLInputElement>('input[type="checkbox"]')?.checked,
  );
}

// A single list-item node represents both ordinary bullet items and GFM task
// items. Tiptap's stock TaskList/TaskItem pair uses a separate list node, which
// cannot represent a Markdown list that mixes `- item` and `- [ ] task` rows.
// Keeping the task state on listItem lets mixed and nested lists round-trip
// without splitting one Markdown list into surprising parent/child structures.
const VisualListItem = ListItem.extend({
  addAttributes() {
    return {
      task: {
        default: false,
        keepOnSplit: true,
        parseHTML: (element: HTMLElement) =>
          element.hasAttribute("data-task-item"),
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.task ? { "data-task-item": "" } : {},
      },
      checked: {
        default: false,
        keepOnSplit: false,
        parseHTML: checkedFromHTML,
        renderHTML: (attributes: Record<string, unknown>) =>
          attributes.task
            ? { "data-checked": String(Boolean(attributes.checked)) }
            : {},
      },
    };
  },

  parseHTML() {
    return [
      {
        tag: "li[data-task-item]",
        priority: 51,
        contentElement: (element: HTMLElement) =>
          element.querySelector(".visual-task-content") ?? element,
      },
      { tag: "li" },
    ];
  },

  renderHTML({ node, HTMLAttributes }: any) {
    if (!node.attrs?.task)
      return [
        "li",
        mergeAttributes(this.options.HTMLAttributes, HTMLAttributes),
        0,
      ];

    const checked = Boolean(node.attrs.checked);
    return [
      "li",
      mergeAttributes(this.options.HTMLAttributes, HTMLAttributes, {
        class: "visual-task-item",
        "data-task-item": "",
        "data-checked": String(checked),
      }),
      [
        "label",
        { class: "visual-task-checkbox", contenteditable: "false" },
        [
          "input",
          {
            type: "checkbox",
            checked: checked ? "checked" : null,
            "aria-label": taskCheckboxLabel(node),
          },
        ],
      ],
      ["div", { class: "visual-task-content" }, 0],
    ];
  },

  addNodeView() {
    return ({ node, HTMLAttributes, getPos, editor }: any) => {
      let current = node;
      const task = Boolean(node.attrs?.task);
      const listItem = document.createElement("li");

      applyDOMAttributes(
        listItem,
        this.options.HTMLAttributes as Record<string, unknown>,
        HTMLAttributes,
      );

      if (!task) {
        return {
          dom: listItem,
          contentDOM: listItem,
          update(updated: any) {
            if (updated.type !== current.type || Boolean(updated.attrs?.task))
              return false;
            current = updated;
            return true;
          },
        };
      }

      listItem.classList.add("visual-task-item");
      listItem.dataset.taskItem = "";

      const label = document.createElement("label");
      label.className = "visual-task-checkbox";
      label.contentEditable = "false";

      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";

      const content = document.createElement("div");
      content.className = "visual-task-content";

      const syncCheckbox = () => {
        const checked = Boolean(current.attrs?.checked);
        listItem.dataset.checked = String(checked);
        checkbox.checked = checked;
        checkbox.setAttribute("aria-label", taskCheckboxLabel(current));
      };

      checkbox.addEventListener("mousedown", (event) => event.preventDefault());
      checkbox.addEventListener("change", () => {
        const position = typeof getPos === "function" ? getPos() : null;
        if (typeof position !== "number") return;
        const currentNode = editor.state.doc.nodeAt(position);
        if (!currentNode || currentNode.type !== current.type) return;

        editor.view.dispatch(
          editor.state.tr.setNodeMarkup(position, undefined, {
            ...currentNode.attrs,
            task: true,
            checked: checkbox.checked,
          }),
        );
      });

      label.append(checkbox);
      listItem.append(label, content);
      syncCheckbox();

      return {
        dom: listItem,
        contentDOM: content,
        update(updated: any) {
          if (updated.type !== current.type || !Boolean(updated.attrs?.task))
            return false;
          current = updated;
          syncCheckbox();
          return true;
        },
      };
    };
  },

  parseMarkdown(
    token: MarkdownToken,
    helpers: MarkdownParseHelpers,
  ): MarkdownParseResult {
    if (!baseParseMarkdown) return [];

    // Marked prepends a synthetic checkbox token to task list items. The stock
    // list-item parser treats that as block content and leaves the following
    // inline text outside a paragraph, which is invalid for a listItem node.
    // Remove only the marker: task state is retained separately in the attrs.
    const normalizedToken = token.task
      ? {
          ...token,
          tokens: token.tokens?.filter((child) => child.type !== "checkbox"),
        }
      : token;
    const parsed = baseParseMarkdown.call(this, normalizedToken, helpers);
    if (Array.isArray(parsed) || "mark" in parsed) return parsed;

    const task = Boolean(token.task);
    return {
      ...parsed,
      attrs: {
        ...(parsed.attrs || {}),
        task,
        checked: task && Boolean(token.checked),
      },
    };
  },

  renderMarkdown(node: any, helpers: any, context: any) {
    if (!node.attrs?.task)
      return baseRenderMarkdown?.call(this, node, helpers, context) ?? "";

    return renderNestedMarkdownContent(
      node,
      helpers,
      `- [${node.attrs.checked ? "x" : " "}] `,
      context,
    );
  },
});

export function visualTaskListExtensions() {
  return [VisualListItem] as const;
}

export function isVisualTaskListInsert(
  action: VisualTaskListInsert | undefined,
): boolean {
  return Boolean(
    action &&
    (action.mode || "insert") === "prefix-lines" &&
    /^\s*[-+*]\s+\[[ xX]\]\s+$/.test(action.markdown),
  );
}

function selectedListItems(editor: VisualListEditor): Array<{
  node: any;
  position: number;
}> {
  const selection = editor.state?.selection;
  const documentNode = editor.state?.doc;
  if (!selection || !documentNode) return [];

  const found = new Map<number, any>();
  const addAncestor = ($position: any) => {
    if (!$position) return;
    for (let depth = $position.depth; depth > 0; depth -= 1) {
      const node = $position.node(depth);
      if (node?.type?.name !== "listItem") continue;
      found.set($position.before(depth), node);
      break;
    }
  };

  addAncestor(selection.$from);
  addAncestor(selection.$to);

  if (!selection.empty) {
    documentNode.nodesBetween(
      selection.from,
      selection.to,
      (node: any, position: number) => {
        if (node?.type?.name === "listItem") found.set(position, node);
        return true;
      },
    );
  }

  return [...found.entries()]
    .sort(([left], [right]) => left - right)
    .map(([position, node]) => ({ position, node }));
}

// Converts the current paragraph/list selection between ordinary bullet items
// and task items. This is intentionally structural instead of inserting the
// literal `- [ ] ` string into ProseMirror: literal insertion inside an
// existing list creates an empty parent bullet with a nested task list.
export function toggleVisualTaskList(editor: VisualListEditor): boolean {
  if (!editor) return false;

  if (!editor.isActive("bulletList")) {
    const converted = editor.chain().focus().toggleBulletList().run();
    if (!converted) return false;
  }

  const items = selectedListItems(editor);
  if (!items.length) return false;

  const removeTasks = items.every(({ node }) => Boolean(node.attrs?.task));
  const transaction = editor.state.tr;

  for (const { node, position } of items) {
    transaction.setNodeMarkup(position, undefined, {
      ...node.attrs,
      task: !removeTasks,
      checked: removeTasks
        ? false
        : node.attrs?.task
          ? Boolean(node.attrs.checked)
          : false,
    });
  }

  if (!transaction.docChanged) return false;
  editor.view.dispatch(transaction);
  editor.view.focus();
  return true;
}
