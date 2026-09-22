export function visualPane(workspace: HTMLElement): HTMLElement {
  const existing = workspace.querySelector<HTMLElement>(
    "[data-visual-editor-pane]",
  );
  if (existing) return existing;

  const pane = document.createElement("section");
  pane.className = "editor-visual-pane";
  pane.dataset.visualEditorPane = "";
  pane.hidden = true;
  pane.innerHTML =
    '<div class="visual-editor-status" data-visual-status aria-live="polite">Visual editor loads when opened.</div>' +
    '<div class="visual-editor-surface" data-visual-editor></div>' +
    '<div class="visual-slash-menu" data-visual-slash-menu role="listbox" aria-label="Insert block" hidden></div>';

  const preview = workspace.querySelector("[data-editor-preview]");
  if (preview) preview.before(pane);
  else workspace.append(pane);
  return pane;
}
