// Editor mode switching and Markdown preview behavior.

import { createLatestRequest, isAbortError } from "../../core/async.ts";
import { requiredAttribute } from "../../core/dom.ts";
import { isRecord } from "../../core/guards.ts";
import { errorMessage, requestJSON } from "../../core/http.ts";
import { setupMarkdownEnhancements } from "../markdown.ts";
import { renderPluginModules } from "../../plugins/loader.ts";
import { preferredEditorMode, rememberEditorMode } from "./experience.ts";

export type EditorMode = "write" | "visual" | "split";

const modes = new Set<EditorMode>(["write", "visual", "split"]);

interface PreviewPayload {
  html: string;
}

function isPreviewPayload(value: unknown): value is PreviewPayload {
  return isRecord(value) && typeof value.html === "string";
}

function editorMode(value: string | undefined): EditorMode {
  return value && modes.has(value as EditorMode)
    ? (value as EditorMode)
    : "write";
}

// Returns labels for an editor workspace mode.
export function editorModeCopy(mode: string): {
  title: string;
  description: string;
} {
  switch (mode) {
    case "visual":
      return {
        title: "Visual",
        description:
          "Edit the page directly while Markdown remains the source of truth.",
      };
    case "split":
      return {
        title: "Markdown",
        description: "Live preview is open beside the Markdown source.",
      };
    default:
      return {
        title: "Markdown",
        description: "Edit the Markdown source directly.",
      };
  }
}

// Wires editor mode and Markdown preview behavior.
function setupEditorPreview(form: HTMLFormElement): void {
  const workspace = form.querySelector<HTMLElement>("[data-editor-workspace]");
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const slug = form.querySelector<HTMLInputElement>('[name="slug"]');
  const visual = form.querySelector<HTMLElement>("[data-visual-editor-pane]");
  const preview = form.querySelector<HTMLElement>("[data-editor-preview]");
  const content = form.querySelector<HTMLElement>(
    "[data-editor-preview-content]",
  );
  const status = form.querySelector<HTMLElement>(
    "[data-editor-preview-status]",
  );
  const previewToggle = form.querySelector<HTMLButtonElement>(
    "[data-editor-preview-toggle]",
  );
  const sectionTitle = form.querySelector<HTMLElement>(
    "[data-editor-section-title]",
  );
  const sectionDescription = form.querySelector<HTMLElement>(
    "[data-editor-section-description]",
  );
  const buttons = [
    ...form.querySelectorAll<HTMLButtonElement>("button[data-editor-mode]"),
  ];
  if (
    !workspace ||
    !source ||
    !preview ||
    !content ||
    !status ||
    !buttons.length
  )
    return;

  const previewEndpoint = requiredAttribute(form, "data-preview-url");
  const editorWorkspace = workspace;
  const sourceEditor = source;
  const visualPanel = visual;
  const previewPanel = preview;
  let previewContent = content;
  const previewStatus = status;
  const previewRequests = createLatestRequest();
  let timer: ReturnType<typeof setTimeout> | undefined;
  let mode: EditorMode = "write";
  let syncing = false;
  let renderedInput: string | undefined;
  let pendingInput: string | undefined;
  let renderedHTML: string | undefined;

  function resolvedBlueprintMarkdown(markdown: string): string {
    let resolved = markdown;
    const fields = form.querySelectorAll<HTMLInputElement>(
      "input[data-blueprint-field]",
    );
    for (const field of fields) {
      const name = field.dataset.blueprintField;
      if (!name) continue;
      resolved = resolved.split(`{{field:${name}}}`).join(field.value);
    }
    return resolved;
  }

  function previewInput(): string {
    return JSON.stringify({
      markdown: resolvedBlueprintMarkdown(sourceEditor.value),
      slug: slug?.value || "",
    });
  }

  // Renders the Markdown preview without flashing stale intermediate content.
  async function renderPreview(): Promise<void> {
    timer = undefined;
    const input = previewInput();
    if (input === renderedInput) {
      previewStatus.hidden = true;
      return;
    }
    if (input === pendingInput) return;

    const signal = previewRequests.next();
    pendingInput = input;
    let staging: HTMLDivElement | undefined;

    // Background refreshes must not insert a status row and shift the content.
    previewStatus.hidden = renderedInput !== undefined;
    previewStatus.textContent = "Rendering preview…";

    try {
      const payload = await requestJSON(previewEndpoint, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: input,
        signal,
      });
      if (!isPreviewPayload(payload))
        throw new Error("Invalid preview response.");
      if (signal.aborted) return;

      if (payload.html !== renderedHTML) {
        // Browser modules need a connected, measurable element. Prepare it offscreen
        // so raw diagram source never replaces the currently visible preview.
        staging = document.createElement("div");
        staging.className = `${previewContent.className} editor-preview-staging`;
        staging.style.width = `${previewContent.getBoundingClientRect().width}px`;
        staging.setAttribute("aria-hidden", "true");
        staging.inert = true;
        staging.innerHTML = payload.html;
        previewPanel.append(staging);
        setupMarkdownEnhancements(staging);
        await renderPluginModules(staging);
        if (signal.aborted) return;

        const scrollTop = previewPanel.scrollTop;
        // Keep the staged node in place so moving an iframe cannot restart it.
        previewContent.remove();
        staging.className = previewContent.className;
        staging.style.width = "";
        staging.removeAttribute("aria-hidden");
        staging.inert = false;
        staging.setAttribute("data-editor-preview-content", "");
        previewContent = staging;
        staging = undefined;
        previewPanel.scrollTop = scrollTop;
        renderedHTML = payload.html;
      }
      renderedInput = input;
      previewStatus.hidden = true;
    } catch (error) {
      if (signal.aborted || isAbortError(error)) return;

      console.error("markdown preview failed", error);
      previewStatus.hidden = false;
      previewStatus.textContent =
        errorMessage(error) || "Preview could not be rendered.";
    } finally {
      staging?.remove();
      if (previewRequests.current() === signal) pendingInput = undefined;
    }
  }

  // Schedules preview only while the Markdown split view is open.
  function schedulePreview(): void {
    if (previewInput() === pendingInput) return;

    if (timer !== undefined) clearTimeout(timer);
    // Invalidate old responses immediately, including during the debounce.
    previewRequests.abort();
    pendingInput = undefined;
    if (mode !== "split") return;

    timer = setTimeout(() => void renderPreview(), 180);
  }

  // Sets the primary editor mode or the Markdown split submode.
  function setMode(nextMode: string | undefined, remember = true): void {
    if (timer !== undefined) clearTimeout(timer);
    timer = undefined;
    mode = editorMode(nextMode);
    form.dataset.editorMode = mode;
    editorWorkspace.dataset.editorMode = mode;
    if (visualPanel) visualPanel.hidden = mode !== "visual";
    previewPanel.hidden = mode !== "split";

    const copy = editorModeCopy(mode);

    if (sectionTitle) sectionTitle.textContent = copy.title;
    if (sectionDescription) sectionDescription.textContent = copy.description;
    for (const button of buttons) {
      const buttonMode = button.dataset.editorMode;
      const active =
        buttonMode === mode || (buttonMode === "write" && mode === "split");

      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    }

    if (previewToggle) {
      previewToggle.hidden = mode === "visual";
      previewToggle.classList.toggle("active", mode === "split");
      previewToggle.setAttribute("aria-pressed", String(mode === "split"));
    }

    if (remember) rememberEditorMode(mode);
    form.dispatchEvent(new CustomEvent("editor:mode-change"));
    if (mode === "visual") {
      previewRequests.abort();
      pendingInput = undefined;
      form.dispatchEvent(new CustomEvent("editor:visual-activate"));
    } else if (mode === "split") void renderPreview();
    else {
      previewRequests.abort();
      pendingInput = undefined;
    }
  }

  // Synchronizes source and preview scrolling in split mode.
  function syncScroll(from: HTMLElement, to: HTMLElement): void {
    if (mode !== "split" || syncing) return;

    const fromRange = from.scrollHeight - from.clientHeight;
    const toRange = to.scrollHeight - to.clientHeight;
    if (fromRange <= 0 || toRange <= 0) return;

    syncing = true;
    to.scrollTop = (from.scrollTop / fromRange) * toRange;
    requestAnimationFrame(() => (syncing = false));
  }

  for (const button of buttons) {
    button.addEventListener("click", () => setMode(button.dataset.editorMode));
  }

  previewToggle?.addEventListener("click", () =>
    setMode(mode === "split" ? "write" : "split"),
  );
  form.addEventListener("editor:toggle-preview", () =>
    setMode(mode === "split" ? "write" : "split"),
  );
  sourceEditor.addEventListener("input", schedulePreview);
  slug?.addEventListener("input", schedulePreview);
  for (const field of form.querySelectorAll<HTMLInputElement>(
    "input[data-blueprint-field]",
  )) {
    field.addEventListener("input", schedulePreview);
  }
  sourceEditor.addEventListener("scroll", () =>
    syncScroll(sourceEditor, previewPanel),
  );
  previewPanel.addEventListener("scroll", () =>
    syncScroll(previewPanel, sourceEditor),
  );

  setMode(preferredEditorMode(), false);
}

// Initializes editor mode and Markdown preview behavior.
export function initEditorPreview(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupEditorPreview(form);
}
