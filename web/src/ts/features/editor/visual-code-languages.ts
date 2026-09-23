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
import { codeLanguageLabel } from "./code-languages.ts";
import { openCodeLanguagePicker } from "./code-language-picker.ts";

// visualCodeLanguages decorates existing codeBlock nodes without replacing their schema or node view.
export function visualCodeLanguages(): AnyExtension {
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
                        button.textContent = codeLanguageLabel(language);
                        button.setAttribute(
                          "aria-label",
                          `Code language: ${codeLanguageLabel(language)}. Change language`,
                        );
                        button.title = "Change code block language";

                        button.addEventListener("mousedown", (event) => {
                          event.preventDefault();
                          event.stopPropagation();
                        });
                        button.addEventListener("click", (event) => {
                          event.preventDefault();
                          event.stopPropagation();
                          void openCodeLanguagePicker(language).then((next) => {
                            if (next === null || next === language) return;
                            const current = view.state.doc.nodeAt(position);
                            if (!current || current.type.name !== "codeBlock")
                              return;
                            view.dispatch(
                              view.state.tr.setNodeMarkup(position, undefined, {
                                ...current.attrs,
                                language: next || null,
                              }),
                            );
                          });
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
