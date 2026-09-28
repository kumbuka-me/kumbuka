// Tiptap extensions and commands used by the visual editor.

import {
  Extension,
  Node as TiptapNode,
  Plugin,
  Decoration,
  DecorationSet,
  type AnyExtension,
} from "./visual-deps/core.ts";
import type { EditorInsertAction } from "./toolbar.ts";
import {
  markdownTables,
  parseTableDirective,
  type TableDirective,
} from "./tables.ts";
import {
  matchKumbukaBlockSyntax,
  matchKumbukaInlineSyntax,
  visualSyntaxLabel,
} from "./visual-syntax.ts";
import { matchWidgetSource, type CatalogWidget } from "./widget-contract.ts";
import { mentionRanges } from "../mentions.ts";

export interface VisualEditor {
  chain(): any;
  commands: {
    insertContent(content: string, options?: Record<string, unknown>): boolean;
    setContent(content: string, options?: Record<string, unknown>): boolean;
  };
  destroy(): void;
  getAttributes(name: string): Record<string, unknown>;
  getMarkdown(): string;
  isActive(name: string, attributes?: Record<string, unknown>): boolean;
  state: any;
  view: any;
}

interface SlashCommand {
  id: string;
  label: string;
  keywords: string;
  run(editor: VisualEditor, from: number, to: number): void;
}

function firstFallbackInlineIndex(
  source: string,
  widgets: CatalogWidget[],
): number {
  let offset = 0;

  while (offset < source.length) {
    const macro = source.indexOf("{{", offset);
    const wiki = source.indexOf("[[", offset);
    const index = macro < 0 ? wiki : wiki < 0 ? macro : Math.min(macro, wiki);
    if (index < 0) return -1;

    const raw = matchKumbukaInlineSyntax(source.slice(index));
    if (!raw) {
      offset = index + 2;
      continue;
    }
    if (raw.startsWith("{{")) {
      const matched = matchWidgetSource(raw, widgets, true);
      if (matched?.raw === raw) {
        offset = index + raw.length;
        continue;
      }
    }
    return index;
  }

  return -1;
}

function fallbackInlineNodeView(context: any): any {
  let node = context.node;
  const dom = document.createElement("span");
  dom.className = "visual-kumbuka-token visual-kumbuka-fallback";
  dom.contentEditable = "false";
  dom.dataset.kumbukaInline = "";

  const label = document.createElement("span");
  const edit = document.createElement("button");
  edit.type = "button";
  edit.className = "visual-kumbuka-edit-source";
  edit.textContent = "Edit source";
  edit.title = "Edit Markdown source";
  dom.append(label, edit);

  const render = () => {
    const raw = String(node.attrs?.raw || "");
    label.textContent = visualSyntaxLabel(raw);
    dom.dataset.kumbukaRaw = raw;
    dom.title = raw;
  };
  edit.addEventListener("mousedown", (event) => event.preventDefault());
  edit.addEventListener("click", (event) => {
    event.preventDefault();
    event.stopPropagation();
    const raw = String(node.attrs?.raw || "");
    const source = window.prompt("Edit source", raw);
    if (source === null || source === raw) return;
    const parsed = matchKumbukaInlineSyntax(source);
    if (parsed !== source) {
      window.alert(
        "The edited source must remain one complete inline Kumbuka construct.",
      );
      return;
    }
    const position = context.getPos();
    if (typeof position !== "number") return;
    context.editor.view.dispatch(
      context.editor.state.tr.setNodeMarkup(position, undefined, {
        ...node.attrs,
        raw: source,
      }),
    );
  });
  render();

  return {
    dom,
    update(updated: any) {
      if (updated.type !== node.type) return false;
      node = updated;
      render();
      return true;
    },
    selectNode() {
      dom.classList.add("ProseMirror-selectednode");
    },
    deselectNode() {
      dom.classList.remove("ProseMirror-selectednode");
    },
    stopEvent(event: Event) {
      return (
        event.target instanceof globalThis.Node && edit.contains(event.target)
      );
    },
    ignoreMutation() {
      return true;
    },
  };
}

export function kumbukaInlineNode(widgets: CatalogWidget[]): AnyExtension {
  return TiptapNode.create({
    name: "kumbukaInline",
    inline: true,
    group: "inline",
    atom: true,
    selectable: true,
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
      return [{ tag: "span[data-kumbuka-inline]" }];
    },
    addNodeView() {
      return (context: any) => fallbackInlineNodeView(context);
    },
    renderHTML({ node }: any) {
      const raw = String(node.attrs?.raw || "");
      return [
        "span",
        {
          class: "visual-kumbuka-token",
          "data-kumbuka-inline": "",
          "data-kumbuka-raw": raw,
          title: raw,
        },
        visualSyntaxLabel(raw),
      ];
    },
    markdownTokenName: "kumbuka_inline",
    markdownTokenizer: {
      name: "kumbuka_inline",
      level: "inline",
      start(source: string) {
        return firstFallbackInlineIndex(source, widgets);
      },
      tokenize(source: string) {
        const raw = matchKumbukaInlineSyntax(source);
        if (!raw) return undefined;
        if (raw.startsWith("{{")) {
          const matched = matchWidgetSource(raw, widgets, true);
          if (matched?.raw === raw) return undefined;
        }
        return { type: "kumbuka_inline", raw, text: raw };
      },
    },
    parseMarkdown(token: any) {
      return {
        type: "kumbukaInline",
        attrs: { raw: String(token.raw || token.text || "") },
      };
    },
    renderMarkdown(node: any) {
      return String(node.attrs?.raw || "");
    },
  });
}

export function kumbukaBlockNode(): AnyExtension {
  return TiptapNode.create({
    name: "kumbukaBlock",
    group: "block",
    atom: true,
    selectable: true,
    draggable: true,
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
      return [{ tag: "div[data-kumbuka-block]" }];
    },
    renderHTML({ node }: any) {
      const raw = String(node.attrs?.raw || "");
      return [
        "div",
        {
          class: "visual-kumbuka-block",
          ...(parseTableDirective(raw)
            ? { hidden: "", "data-visual-table-options": "" }
            : {}),
          "data-kumbuka-block": "",
          "data-kumbuka-raw": raw,
          title: raw,
        },
        visualSyntaxLabel(raw),
      ];
    },
    markdownTokenName: "kumbuka_block",
    markdownTokenizer: {
      name: "kumbuka_block",
      level: "block",
      start(source: string) {
        const match =
          /(^|\n)\{[A-Za-z][A-Za-z0-9_-]*(?:[ \t]+[^{}\n]*)?\}(?=\n|$)/.exec(
            source,
          );
        if (!match) return -1;
        return match.index + (match[1] ? 1 : 0);
      },
      tokenize(source: string) {
        const raw = matchKumbukaBlockSyntax(source);
        if (!raw) return undefined;
        const consumed = source.startsWith(raw + "\n") ? raw + "\n" : raw;
        return { type: "kumbuka_block", raw: consumed, text: raw };
      },
    },
    parseMarkdown(token: any) {
      return {
        type: "kumbukaBlock",
        attrs: {
          raw: String(token.text || token.raw || "").replace(/\n$/, ""),
        },
      };
    },
    renderMarkdown(node: any) {
      const raw = String(node.attrs?.raw || "");
      return raw ? raw + "\n\n" : "";
    },
  });
}

export function pluginMarkdown(
  action: EditorInsertAction,
  editor: VisualEditor,
): string {
  const markdown = action.markdown || "";
  const suffix = action.suffix || "";
  const placeholder = action.placeholder || "";
  const selection = editor.state?.selection;
  const from = Number(selection?.from || 0);
  const to = Number(selection?.to || from);
  const selected =
    to > from
      ? String(editor.state.doc.textBetween(from, to, "\n", "\n") || "")
      : "";

  switch (action.mode || "insert") {
    case "wrap":
      return markdown + (selected || placeholder || "text") + suffix;
    case "prefix-lines":
      return (selected || placeholder || "item")
        .split("\n")
        .map((line: string) => markdown + line)
        .join("\n");
    default:
      return markdown + suffix;
  }
}

interface VisualTableContext {
  tableIndex: number;
  kind: "header" | "body";
  row: number;
  column: number;
}

export function currentVisualTableContext(
  editor: VisualEditor,
): VisualTableContext | null {
  const selection = editor.state?.selection;
  const $from = selection?.$from;
  if (!$from) return null;

  let tableDepth = -1;
  let rowDepth = -1;
  let cellDepth = -1;

  for (let depth = $from.depth; depth > 0; depth -= 1) {
    const name = $from.node(depth)?.type?.name;
    if (cellDepth < 0 && (name === "tableCell" || name === "tableHeader"))
      cellDepth = depth;
    else if (rowDepth < 0 && name === "tableRow") rowDepth = depth;
    else if (tableDepth < 0 && name === "table") {
      tableDepth = depth;
      break;
    }
  }

  if (tableDepth < 0 || rowDepth < 0 || cellDepth < 0) return null;

  const tableNode = $from.node(tableDepth);
  const rowNode = $from.node(rowDepth);
  const cellNode = $from.node(cellDepth);
  const rowIndex = $from.index(tableDepth);
  const columnIndex = $from.index(rowDepth);
  const tablePosition = $from.before(tableDepth);
  let tableIndex = 0;

  editor.state.doc.descendants((node: any, position: number) => {
    if (node.type?.name !== "table") return true;
    if (position < tablePosition) tableIndex += 1;
    return false;
  });

  if (cellNode.type?.name === "tableHeader") {
    return {
      tableIndex,
      kind: "header",
      row: 0,
      column: columnIndex + 1,
    };
  }

  let bodyRow = 0;
  for (let index = 0; index <= rowIndex; index += 1) {
    const firstCell = tableNode.child(index)?.firstChild;
    if (firstCell?.type?.name !== "tableHeader") bodyRow += 1;
  }

  return {
    tableIndex,
    kind: "body",
    row: Math.max(1, bodyRow),
    column: Math.max(1, Math.min(columnIndex + 1, rowNode.childCount || 1)),
  };
}

function directiveToneForCell(
  directive: TableDirective,
  header: boolean,
  row: number,
  column: number,
): string {
  if (!header) {
    const cell = directive.cells?.[`${row},${column}`];
    if (cell) return cell;
    const rowTone = directive.rows?.[String(row)];
    if (rowTone) return rowTone;
  }

  const columnTone = directive.columns?.[String(column)];
  if (columnTone) return columnTone;
  return header ? directive.header || "" : "";
}

export function visualTableStyles(source: () => string): AnyExtension {
  return Extension.create({
    name: "visualTableStyles",
    addGlobalAttributes() {
      return [
        {
          types: ["tableRow"],
          attributes: {
            height: {
              default: null,
              renderHTML: (attrs) =>
                attrs.height ? { style: `height: ${attrs.height}px` } : {},
            },
          },
        },
      ];
    },
    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            decorations(state) {
              const tables = markdownTables(source());
              const decorations: Decoration[] = [];
              let tableIndex = 0;
              state.doc.descendants((table, position) => {
                if (table.type.name !== "table") return true;
                const directive = tables[tableIndex++]?.directive;

                let bodyRow = 0;
                table.forEach((row, rowOffset) => {
                  const header = row.firstChild?.type.name === "tableHeader";
                  if (!header) bodyRow += 1;
                  row.forEach((cell, cellOffset, column) => {
                    const rowPosition = position + 1 + rowOffset;
                    const cellPosition = rowPosition + 1 + cellOffset;
                    decorations.push(
                      Decoration.widget(
                        cellPosition + cell.nodeSize - 1,
                        (view) => {
                          const handle = document.createElement("span");
                          handle.className = "row-resize-handle";
                          handle.contentEditable = "false";
                          handle.title = "Drag to resize row";
                          handle.addEventListener("mousedown", (event) => {
                            event.preventDefault();
                            event.stopPropagation();
                            const startY = event.clientY;
                            const rowElement = view.nodeDOM(
                              rowPosition,
                            ) as HTMLElement;
                            const height =
                              rowElement.getBoundingClientRect().height;
                            const move = (moveEvent: MouseEvent) => {
                              const current =
                                view.state.doc.nodeAt(rowPosition);
                              if (!current || current.type.name !== "tableRow")
                                return;
                              const next = Math.max(
                                28,
                                Math.min(
                                  4000,
                                  Math.round(
                                    height + moveEvent.clientY - startY,
                                  ),
                                ),
                              );
                              view.dispatch(
                                view.state.tr.setNodeMarkup(
                                  rowPosition,
                                  undefined,
                                  { ...current.attrs, height: next },
                                ),
                              );
                            };
                            const up = () => {
                              window.removeEventListener("mousemove", move);
                              window.removeEventListener("mouseup", up);
                            };
                            window.addEventListener("mousemove", move);
                            window.addEventListener("mouseup", up, {
                              once: true,
                            });
                          });
                          return handle;
                        },
                        {
                          key: `row-resize-${cellPosition}`,
                          ignoreSelection: true,
                        },
                      ),
                    );
                    if (!directive) return;
                    const tone = directiveToneForCell(
                      directive,
                      header,
                      bodyRow,
                      column + 1,
                    );
                    if (!tone) return;
                    const start = position + 2 + rowOffset + cellOffset;
                    decorations.push(
                      Decoration.node(start, start + cell.nodeSize, {
                        class: `table-tone-${tone}`,
                      }),
                    );
                  });
                });
                return false;
              });
              return DecorationSet.create(state.doc, decorations);
            },
          },
        }),
      ];
    },
  });
}

// Styles Markdown mentions like Confluence person lozenges without changing source.
export function visualMentions(): AnyExtension {
  return Extension.create({
    name: "visualMentions",
    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            decorations(state) {
              const decorations: Decoration[] = [];

              state.doc.descendants((node, position, parent) => {
                if (!node.isText || !node.text) return true;
                if (parent?.type?.name === "codeBlock") return false;
                if (node.marks?.some((mark: any) => mark.type.name === "code"))
                  return true;

                for (const mention of mentionRanges(node.text)) {
                  decorations.push(
                    Decoration.inline(
                      position + mention.start,
                      position + mention.end,
                      {
                        class: "visual-mention",
                        "data-visual-mention": mention.username,
                        title: `Mention @${mention.username}`,
                      },
                    ),
                  );
                }
                return true;
              });

              return DecorationSet.create(state.doc, decorations);
            },
          },
        }),
      ];
    },
  });
}

export function slashCommands(): SlashCommand[] {
  const withDelete =
    (
      action: (chain: any) => any,
    ): ((editor: VisualEditor, from: number, to: number) => void) =>
    (editor, from, to) => {
      action(editor.chain().focus().deleteRange({ from, to })).run();
    };

  return [
    {
      id: "paragraph",
      label: "Text",
      keywords: "paragraph text normal",
      run: withDelete((chain) => chain.setParagraph()),
    },
    {
      id: "heading-1",
      label: "Heading 1",
      keywords: "heading title h1",
      run: withDelete((chain) => chain.setHeading({ level: 1 })),
    },
    {
      id: "heading-2",
      label: "Heading 2",
      keywords: "heading subtitle h2",
      run: withDelete((chain) => chain.setHeading({ level: 2 })),
    },
    {
      id: "heading-3",
      label: "Heading 3",
      keywords: "heading h3",
      run: withDelete((chain) => chain.setHeading({ level: 3 })),
    },
    {
      id: "bullet-list",
      label: "Bullet list",
      keywords: "list bullet unordered",
      run: withDelete((chain) => chain.toggleBulletList()),
    },
    {
      id: "ordered-list",
      label: "Numbered list",
      keywords: "list ordered numbered",
      run: withDelete((chain) => chain.toggleOrderedList()),
    },
    {
      id: "blockquote",
      label: "Quote",
      keywords: "quote blockquote",
      run: withDelete((chain) => chain.toggleBlockquote()),
    },
    {
      id: "code-block",
      label: "Code block",
      keywords: "code preformatted",
      run: withDelete((chain) => chain.toggleCodeBlock()),
    },
    {
      id: "horizontal-rule",
      label: "Divider",
      keywords: "divider horizontal rule separator",
      run: withDelete((chain) => chain.setHorizontalRule()),
    },
    {
      id: "table",
      label: "Table",
      keywords: "table grid rows columns",
      run: withDelete((chain) =>
        chain.insertTable({ rows: 3, cols: 3, withHeaderRow: true }),
      ),
    },
  ];
}
