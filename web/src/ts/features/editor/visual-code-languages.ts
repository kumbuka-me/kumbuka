import { loadEditorCatalog } from "./catalog.ts";
import { rewriteWidgetSource, widgetValues } from "./widget-contract.ts";
// Non-invasive code-block language controls layered over Tiptap's existing codeBlock node.

import {
  Decoration,
  DecorationSet,
  Extension,
  Plugin,
  type AnyExtension,
  type EditorState,
  type EditorView,
  type ProseMirrorNode,
} from "./visual-deps/core.ts";
import { setupCopyButton } from "../../core/clipboard.ts";
import {
  codeLanguageLabel,
  pluginCodeLanguages,
  type CodeLanguage,
} from "./code-languages.ts";
import { openCodeLanguagePicker } from "./code-language-picker.ts";

// visualCodeLanguages decorates existing codeBlock nodes without replacing their schema or node view.
export function visualCodeLanguages(): AnyExtension {
  let extraLanguages: CodeLanguage[] = [];
  void loadEditorCatalog()
    .then((catalog) => {
      extraLanguages = pluginCodeLanguages(catalog);
    })
    .catch(() => {});
  return Extension.create({
    name: "visualCodeLanguages",
    addProseMirrorPlugins() {
      return [
        new Plugin({
          props: {
            decorations(state: EditorState) {
              const decorations: Decoration[] = [];

              state.doc.descendants(
                (node: ProseMirrorNode, position: number) => {
                  if (node.type.name !== "codeBlock") return true;

                  const language = String(node.attrs?.language || "");
                  decorations.push(
                    Decoration.node(position, position + node.nodeSize, {
                      class: "visual-code-block",
                    }),
                  );
                  decorations.push(
                    Decoration.widget(
                      position + 1,
                      (view: EditorView) => {
                        const button = document.createElement("button");
                        button.type = "button";
                        button.className = "visual-code-language";
                        button.contentEditable = "false";
                        button.textContent = codeLanguageLabel(
                          language,
                          extraLanguages,
                        );
                        button.setAttribute(
                          "aria-label",
                          `Code language: ${codeLanguageLabel(language, extraLanguages)}. Change language`,
                        );
                        button.title = "Change code block language";

                        button.addEventListener("mousedown", (event) => {
                          event.preventDefault();
                          event.stopPropagation();
                        });
                        button.addEventListener("click", (event) => {
                          event.preventDefault();
                          event.stopPropagation();
                          void openCodeLanguagePicker(language).then(
                            async (next) => {
                              if (next === null || next === language) return;
                              const current = view.state.doc.nodeAt(position);
                              if (!current || current.type.name !== "codeBlock")
                                return;
                              const catalog = await loadEditorCatalog().catch(
                                () => null,
                              );
                              const widget = catalog?.widgets.find(
                                (item) =>
                                  item.syntax.kind === "fence" &&
                                  item.syntax.name === next,
                              );
                              const widgetType =
                                view.state.schema.nodes.kumbukaWidgetBlock;
                              if (widget && widgetType) {
                                const values = widgetValues("", widget);
                                if (current.textContent)
                                  values.body = current.textContent;
                                view.dispatch(
                                  view.state.tr.replaceWith(
                                    position,
                                    position + current.nodeSize,
                                    widgetType.create({
                                      raw: rewriteWidgetSource(
                                        "",
                                        widget,
                                        values,
                                      ),
                                    }),
                                  ),
                                );
                                return;
                              }
                              view.dispatch(
                                view.state.tr.setNodeMarkup(
                                  position,
                                  undefined,
                                  {
                                    ...current.attrs,
                                    language: next || null,
                                  },
                                ),
                              );
                            },
                          );
                        });

                        return button;
                      },
                      {
                        key: `code-language-${position}-${language}`,
                        ignoreSelection: true,
                        side: -1,
                      },
                    ),
                  );
                  decorations.push(
                    Decoration.widget(
                      position + 1,
                      (view: EditorView) => {
                        const button = document.createElement("button");
                        button.type = "button";
                        button.className = "code-copy-button";
                        button.contentEditable = "false";
                        setupCopyButton(
                          button,
                          () =>
                            view.state.doc.nodeAt(position)?.textContent ?? "",
                          "Copy code to clipboard",
                        );
                        return button;
                      },
                      {
                        key: `code-copy-${position}`,
                        ignoreSelection: true,
                        side: 1,
                      },
                    ),
                  );
                  return false;
                },
              );

              return DecorationSet.create(state.doc, decorations);
            },
          },
        }),
      ];
    },
  });
}
