// Keep Tiptap out of the initial editor module graph.
import { loadEditorCatalog } from "./catalog.ts";
import { visualPane } from "./visual-pane.ts";

export function initLazyVisualEditor(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  )) {
    const workspace = form.querySelector<HTMLElement>(
      "[data-editor-workspace]",
    );
    if (!workspace || !form.querySelector('button[data-editor-mode="visual"]'))
      continue;
    const pane = visualPane(workspace);
    const status = pane.querySelector<HTMLElement>("[data-visual-status]")!;
    const toolbar = form.querySelector<HTMLElement>("[data-markdown-toolbar]");
    let ready = false;
    let attempts = 0;
    let pending = false;
    const syncToolbar = () => {
      if (toolbar)
        toolbar.inert = pending && form.dataset.editorMode === "visual";
    };
    async function activate(): Promise<void> {
      if (ready || pending) return;
      pending = true;
      syncToolbar();
      status.hidden = false;
      status.classList.remove("error");
      status.textContent = "Loading visual editor…";
      try {
        // A failed module fetch can be cached by the browser, so retries use a fresh URL.
        const module =
          attempts++ === 0
            ? await import("./visual.ts")
            : await import(`./visual.js?retry=${attempts}`);
        module.setupVisualEditor(form, loadEditorCatalog);
        ready = true;
        if (form.dataset.editorMode === "visual")
          form.dispatchEvent(new CustomEvent("editor:visual-activate"));
      } catch (error) {
        console.error("visual editor failed to download", error);
        status.classList.add("error");
        status.textContent =
          "Visual editor could not be loaded. You can keep editing in Markdown. ";
        const retry = document.createElement("button");
        retry.type = "button";
        retry.textContent = "Retry";
        retry.addEventListener("click", () => void activate());
        status.append(retry);
      } finally {
        pending = false;
        syncToolbar();
      }
    }
    form.addEventListener("editor:visual-activate", () => void activate());
    form.addEventListener("editor:mode-change", syncToolbar);

    const visualButton = form.querySelector<HTMLButtonElement>(
      'button[data-editor-mode="visual"]',
    );
    visualButton?.addEventListener("click", () => {
      queueMicrotask(() => {
        if (form.dataset.editorMode === "visual") return;
        form.dataset.editorMode = "visual";
        workspace.dataset.editorMode = "visual";
        pane.hidden = false;
        form.dispatchEvent(new CustomEvent("editor:mode-change"));
        form.dispatchEvent(new CustomEvent("editor:visual-activate"));
      });
    });
  }
}
