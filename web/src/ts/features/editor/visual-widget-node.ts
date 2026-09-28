// Generic Tiptap NodeViews for plugin-provided declarative visual-editor widgets.

import { Node as TiptapNode, type AnyExtension } from "./visual-deps/core.ts";
import type { CatalogCompletion } from "./catalog.ts";
import { isRecord } from "../../core/guards.ts";
import { requestJSON } from "../../core/http.ts";
import { createSettingsPopover } from "./visual-widget-form.ts";
import { renderWidget, resetPreview } from "./visual-widget-preview.ts";
import {
  matchWidgetSource,
  widgetForSource,
  type CatalogWidget,
} from "./widget-contract.ts";

// Reports whether a source position starts a widget valid for the requested inline mode.
function isWidgetSourceCandidate(
  source: string,
  index: number,
  widgets: CatalogWidget[],
  inline: boolean,
): boolean {
  const startsAtBlockBoundary =
    inline || index === 0 || source[index - 1] === "\n";
  return (
    startsAtBlockBoundary &&
    Boolean(matchWidgetSource(source.slice(index), widgets, inline))
  );
}

interface WidgetNodeViewContext {
  editor: any;
  getPos: () => number | undefined;
  node: any;
}

const sourceMarkers = ["{{", "!!! ", "???", '=== "'];

function firstWidgetSourceIndex(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): number {
  let offset = 0;

  while (offset < source.length) {
    let index = -1;
    for (const marker of sourceMarkers) {
      const candidate = source.indexOf(marker, offset);
      if (candidate >= 0 && (index < 0 || candidate < index)) index = candidate;
    }
    if (index < 0) return -1;
    if (!isWidgetSourceCandidate(source, index, widgets, inline)) {
      offset = index + 1;
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
  return widgetForSource(raw, widgets);
}


interface RenderedWidgetPayload {
  html: string;
}

function isRenderedWidgetPayload(
  value: unknown,
): value is RenderedWidgetPayload {
  return isRecord(value) && typeof value.html === "string";
}

function widgetNodeView(
  widgets: CatalogWidget[],
  completions: CatalogCompletion[],
  context: WidgetNodeViewContext,
  inline: boolean,
): any {
  let node = context.node;
  let popover: HTMLElement | null = null;
  let renderVersion = 0;
  const shell = document.createElement(inline ? "span" : "div");
  shell.className = inline
    ? "visual-widget-node"
    : "visual-widget-node visual-widget-node-block";
  shell.contentEditable = "false";
  shell.dataset.visualWidget = "";

  const preview = document.createElement(inline ? "span" : "div");
  shell.append(preview);

  const renderServerPreview = async (
    raw: string,
    widget: CatalogWidget,
    version: number,
  ) => {
    if (widget.preview.kind !== "card" || !widget.preview.card.rendered) return;
    const form = shell.closest<HTMLFormElement>("form[data-preview-url]");
    const endpoint = form?.dataset.previewUrl;
    if (!form || !endpoint) return;
    const slug =
      form.querySelector<HTMLInputElement>('[name="slug"]')?.value || "";
    try {
      const payload = await requestJSON(endpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ markdown: raw, slug }),
      });
      if (!isRenderedWidgetPayload(payload) || version !== renderVersion)
        return;
      const staging = document.createElement("div");
      staging.innerHTML = payload.html;
      const rendered = staging.querySelector<HTMLElement>(
        `.${CSS.escape(widget.preview.card.class)}`,
      );
      if (!rendered) return;
      resetPreview(preview);
      preview.append(rendered);
      setupLineAnnotationSelection(rendered, widget);
    } catch {
      // Keep the declarative card fallback when a dynamic preview is unavailable.
    }
  };

  const setupLineAnnotationSelection = (
    rendered: HTMLElement,
    widget: CatalogWidget,
  ) => {
    if (widget.preview.kind !== "card") return;
    const annotation = widget.preview.card.line_annotations;
    if (!annotation) return;
    const rows = [
      ...rendered.querySelectorAll<HTMLElement>(
        `.${CSS.escape(annotation.line_class)}`,
      ),
    ];
    if (!rows.length) return;
    let anchor = -1;
    let selected: { start: number; end: number } | null = null;
    const numberFor = (row: HTMLElement): number =>
      Number.parseInt(
        row.querySelector<HTMLElement>(
          `.${CSS.escape(annotation.line_number_class)}`,
        )?.textContent || "",
        10,
      );
    const toolbar = document.createElement("div");
    toolbar.className = "visual-widget-line-actions";
    toolbar.hidden = true;
    const add = document.createElement("button");
    add.type = "button";
    add.textContent = "Add note";
    toolbar.append(add);
    rendered.append(toolbar);

    const select = (first: number, last: number) => {
      const start = Math.min(first, last);
      const end = Math.max(first, last);
      selected = { start, end };
      for (const row of rows) {
        const number = numberFor(row);
        row.classList.toggle(
          "visual-widget-line-selected",
          number >= start && number <= end,
        );
      }
      add.textContent =
        start === end
          ? `Add note to line ${start}`
          : `Add note to lines ${start}–${end}`;
      toolbar.hidden = false;
    };

    for (const row of rows) {
      const numberControl = row.querySelector<HTMLElement>(
        `.${CSS.escape(annotation.line_number_class)}`,
      );
      numberControl?.classList.add("visual-widget-line-number");
      numberControl?.setAttribute(
        "title",
        "Select this line for an annotation",
      );
      numberControl?.addEventListener("click", (event) => {
        event.preventDefault();
        event.stopPropagation();
        const number = numberFor(row);
        if (!Number.isInteger(number)) return;
        if (!(event instanceof MouseEvent) || !event.shiftKey || anchor < 0)
          anchor = number;
        select(anchor, number);
      });
    }

    rendered.addEventListener("mouseup", () => {
      const selection = window.getSelection();
      if (!selection || selection.isCollapsed || !selection.rangeCount) return;
      const range = selection.getRangeAt(0);
      const elementFor = (node: Node): Element | null =>
        node instanceof Element ? node : node.parentElement;
      const startRow = elementFor(range.startContainer)?.closest<HTMLElement>(
        `.${CSS.escape(annotation.line_class)}`,
      );
      const endRow = elementFor(range.endContainer)?.closest<HTMLElement>(
        `.${CSS.escape(annotation.line_class)}`,
      );
      if (
        !startRow ||
        !endRow ||
        !rendered.contains(startRow) ||
        !rendered.contains(endRow)
      )
        return;
      const start = numberFor(startRow);
      const end = numberFor(endRow);
      if (Number.isInteger(start) && Number.isInteger(end)) {
        anchor = start;
        select(start, end);
      }
    });

    add.addEventListener("click", (event) => {
      event.preventDefault();
      event.stopPropagation();
      if (!selected) return;
      const selection =
        selected.start === selected.end
          ? String(selected.start)
          : `${selected.start}-${selected.end}`;
      open({ attribute: annotation.attribute, selection });
    });
  };

  const render = () => {
    const version = ++renderVersion;
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) {
      preview.className = "visual-kumbuka-token";
      preview.textContent = "Widget";
      preview.title = raw;
      return;
    }
    renderWidget(preview, raw, widget);
    shell.dataset.pluginId = widget.plugin_id;
    shell.dataset.widgetId = widget.id;
    queueMicrotask(() => void renderServerPreview(raw, widget, version));
  };
  const close = (restorePreview = true) => {
    popover?.remove();
    popover = null;
    if (restorePreview) render();
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
    context.editor.view.dispatch(transaction);
  };
  const open = (initialAnnotation?: {
    attribute: string;
    selection: string;
  }) => {
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) return;
    close();
    popover = createSettingsPopover(
      raw,
      widget,
      shell,
      apply,
      (previewRaw) => renderWidget(preview, previewRaw, widget),
      close,
      completions,
      initialAnnotation,
    );
  };

  shell.addEventListener("mousedown", (event) => {
    if (!(event instanceof MouseEvent) || event.button !== 0) return;
    const target = event.target;
    const contract = widgetForSource(String(node.attrs?.raw || ""), widgets);
    const lineNumberClass =
      contract?.preview.kind === "card"
        ? contract.preview.card.line_annotations?.line_number_class
        : undefined;
    const lineClass =
      contract?.preview.kind === "card"
        ? contract.preview.card.line_annotations?.line_class
        : undefined;
    if (
      target instanceof Element &&
      (target.closest(".visual-widget-line-actions") ||
        (lineNumberClass &&
          target.closest(`.${CSS.escape(lineNumberClass)}`)) ||
        (lineClass && target.closest(`.${CSS.escape(lineClass)}`)))
    ) {
      if (
        target.closest(".visual-widget-line-actions") ||
        (lineNumberClass && target.closest(`.${CSS.escape(lineNumberClass)}`))
      )
        event.preventDefault();
      event.stopPropagation();
      return;
    }
    event.preventDefault();
    const position = context.getPos();
    const selection = context.editor.state.selection;
    if (
      typeof position === "number" &&
      (selection.from !== position || selection.to !== position + node.nodeSize)
    )
      context.editor.chain().focus().setNodeSelection(position).run();
    // A selected block widget is draggable, so browsers may suppress its click
    // event. Open from the primary-button press after ProseMirror has applied
    // the node selection instead.
    queueMicrotask(() => open());
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
    update(updated: any) {
      if (updated.type !== node.type) return false;
      node = updated;
      close(false);
      render();
      return true;
    },
    stopEvent(event: Event) {
      return shell.contains(event.target as Node);
    },
    ignoreMutation() {
      return true;
    },
    destroy() {
      close(false);
    },
  };
}

function widgetNode(
  widgets: CatalogWidget[],
  completions: CatalogCompletion[],
  inline: boolean,
): AnyExtension {
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
    renderHTML({ node }: any) {
      return [
        inline ? "span" : "div",
        {
          "data-visual-widget": "",
          "data-kumbuka-raw": String(node.attrs?.raw || ""),
        },
      ];
    },
    addNodeView() {
      return (context: WidgetNodeViewContext) =>
        widgetNodeView(widgets, completions, context, inline);
    },
    markdownTokenName: tokenName,
    markdownTokenizer: {
      name: tokenName,
      level: inline ? "inline" : "block",
      start(source: string) {
        return firstWidgetSourceIndex(source, widgets, inline);
      },
      tokenize(source: string) {
        const matched = matchWidgetSource(source, widgets, inline);
        if (!matched) return undefined;
        const consumed =
          !inline && source.startsWith(matched.raw + "\n")
            ? matched.raw + "\n"
            : matched.raw;
        return { type: tokenName, raw: consumed, text: matched.raw };
      },
    },
    parseMarkdown(token: any) {
      return {
        type: nodeName,
        attrs: {
          raw: String(token.text || token.raw || "").replace(/\n$/, ""),
        },
      };
    },
    renderMarkdown(node: any) {
      const raw = String(node.attrs?.raw || "");
      return raw;
    },
  });
}

// visualWidgetNodes creates the inline and block NodeViews backed by active plugin contracts.
export function visualWidgetNodes(
  widgets: CatalogWidget[],
  completions: CatalogCompletion[] = [],
): AnyExtension[] {
  const extensions: AnyExtension[] = [];
  if (widgets.some((widget) => widget.inline))
    extensions.push(widgetNode(widgets, completions, true));
  if (widgets.some((widget) => !widget.inline))
    extensions.push(widgetNode(widgets, completions, false));
  return extensions;
}
