// Generic Tiptap NodeViews for plugin-provided declarative visual-editor widgets.

import {
  Node as TiptapNode,
  closeHistory,
  type AnyExtension,
} from "./visual-deps/core.ts";
import {
  matchWidgetMacro,
  parseMacro,
  widgetForMacro,
  type CatalogWidget,
} from "./widget-contract.ts";
import { renderWidgetBadge } from "./widget-preview.ts";
import { createWidgetSettings } from "./widget-settings.ts";
import type { NodeViewRendererProps } from "@tiptap/core";
import type { NodeView } from "@tiptap/pm/view";

function firstWidgetMacroIndex(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): number {
  let offset = 0;

  while (offset < source.length) {
    const index = source.indexOf("{{", offset);
    if (index < 0) return -1;
    if (
      (!inline && index > 0 && source[index - 1] !== "\n") ||
      !matchWidgetMacro(source.slice(index), widgets, inline)
    ) {
      offset = index + 2;
      continue;
    }
    return index;
  }

  return -1;
}

function contractForRaw(
  raw: string,
  widgets: CatalogWidget[],
): CatalogWidget | null {
  const macro = parseMacro(raw);
  return macro ? widgetForMacro(macro, widgets) : null;
}

function widgetNodeView(
  widgets: CatalogWidget[],
  context: NodeViewRendererProps,
  inline: boolean,
): NodeView {
  let node = context.node;
  let popover: HTMLElement | null = null;
  let ownerForm: Element | null = null;
  const shell = document.createElement(inline ? "span" : "div");
  shell.className = inline
    ? "visual-widget-node"
    : "visual-widget-node visual-widget-node-block";
  shell.contentEditable = "false";
  shell.dataset.visualWidget = "";
  shell.tabIndex = 0;
  shell.setAttribute("role", "button");
  shell.setAttribute("aria-haspopup", "dialog");

  const preview = document.createElement(inline ? "span" : "div");
  shell.append(preview);

  const onOutsideClick = (event: MouseEvent) => {
    const target = event.target as globalThis.Node;
    if (!shell.contains(target) && !popover?.contains(target)) close();
  };
  const onEscape = (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    event.preventDefault();
    close();
    shell.focus();
  };
  const onModeChange = () => close();
  const close = () => {
    document.removeEventListener("mousedown", onOutsideClick);
    document.removeEventListener("keydown", onEscape);
    ownerForm?.removeEventListener("editor:mode-change", onModeChange);
    ownerForm = null;
    shell.setAttribute("aria-expanded", "false");
    popover?.remove();
    popover = null;
  };
  const render = () => {
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) {
      preview.className = "visual-kumbuka-token";
      preview.textContent = "Widget";
      preview.title = raw;
      return;
    }
    renderWidgetBadge(preview, raw, widget);
    shell.dataset.pluginId = widget.plugin_id;
    shell.dataset.widgetId = widget.id;
    shell.setAttribute("aria-label", `Edit ${widget.name}`);
  };
  const apply = (raw: string) => {
    const position = context.getPos();
    if (typeof position !== "number") return;
    const transaction = context.editor.state.tr.setNodeMarkup(
      position,
      undefined,
      {
        ...node.attrs,
        raw,
      },
    );
    context.editor.view.dispatch(closeHistory(transaction));
  };
  const open = () => {
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) return;
    close();
    popover = createWidgetSettings(raw, widget, shell, apply, () => {
      close();
      shell.focus();
    });
    document.addEventListener("mousedown", onOutsideClick);
    document.addEventListener("keydown", onEscape);
    ownerForm = shell.closest("[data-editor-form]");
    ownerForm?.addEventListener("editor:mode-change", onModeChange);
    shell.setAttribute("aria-expanded", "true");
  };

  shell.addEventListener("click", (event) => {
    event.preventDefault();
    const position = context.getPos();
    if (typeof position === "number")
      context.editor.chain().setNodeSelection(position).run();
    open();
  });
  shell.addEventListener("keydown", (event) => {
    if (
      event instanceof KeyboardEvent &&
      (event.key === "Enter" || event.key === " ")
    ) {
      event.preventDefault();
      shell.click();
    }
  });
  render();

  return {
    dom: shell,
    selectNode() {
      shell.classList.add("ProseMirror-selectednode");
    },
    deselectNode() {
      shell.classList.remove("ProseMirror-selectednode");
      close();
    },
    update(updated) {
      if (updated.eq(node)) return true;
      if (updated.type !== node.type) return false;
      node = updated;
      close();
      render();
      return true;
    },
    stopEvent(event: Event) {
      if (event instanceof KeyboardEvent) return event.defaultPrevented;
      return shell.contains(event.target as Node);
    },
    ignoreMutation() {
      return true;
    },
    destroy: close,
  };
}

function widgetNode(widgets: CatalogWidget[], inline: boolean): AnyExtension {
  const nodeName = inline ? "kumbukaWidgetInline" : "kumbukaWidgetBlock";
  const tokenName = inline ? "kumbuka_widget_inline" : "kumbuka_widget_block";
  return TiptapNode.create({
    name: nodeName,
    priority: 200,
    inline,
    group: inline ? "inline" : "block",
    atom: true,
    selectable: true,
    draggable: !inline,
    addAttributes() {
      return {
        raw: {
          default: "",
          parseHTML: (element: HTMLElement) =>
            element.getAttribute("data-kumbuka-raw") || "",
        },
      };
    },
    parseHTML() {
      return [
        {
          tag: inline ? "span[data-visual-widget]" : "div[data-visual-widget]",
        },
      ];
    },
    renderHTML({ node }) {
      return [
        inline ? "span" : "div",
        {
          "data-visual-widget": "",
          "data-kumbuka-raw": String(node.attrs?.raw || ""),
        },
      ];
    },
    addNodeView() {
      return (context: NodeViewRendererProps) =>
        widgetNodeView(widgets, context, inline);
    },
    markdownTokenName: tokenName,
    markdownTokenizer: {
      name: tokenName,
      level: inline ? "inline" : "block",
      start(source: string) {
        return firstWidgetMacroIndex(source, widgets, inline);
      },
      tokenize(source: string) {
        const matched = matchWidgetMacro(source, widgets, inline);
        if (!matched) return undefined;
        const consumed =
          !inline && source.startsWith(matched.raw + "\n")
            ? matched.raw + "\n"
            : matched.raw;
        return { type: tokenName, raw: consumed, text: matched.raw };
      },
    },
    parseMarkdown(token) {
      return {
        type: nodeName,
        attrs: {
          raw: String(token.text || token.raw || "").replace(/\n$/, ""),
        },
      };
    },
    renderMarkdown(node) {
      const raw = String(node.attrs?.raw || "");
      return inline ? raw : raw ? raw + "\n\n" : "";
    },
  });
}

// visualWidgetNodes creates the inline and block NodeViews backed by active plugin contracts.
export function visualWidgetNodes(widgets: CatalogWidget[]): AnyExtension[] {
  const extensions: AnyExtension[] = [];
  if (widgets.some((widget) => widget.inline))
    extensions.push(widgetNode(widgets, true));
  if (widgets.some((widget) => !widget.inline))
    extensions.push(widgetNode(widgets, false));
  return extensions;
}
