import { slugifyPagePath as slugifyEditorPath } from "../../core/slug.ts";
// Editor drafts, navigation guards, stats, and workspace state.

import { requestConfirmation, showNotice } from "../../core/dialogs.ts";
import { requiredAttribute } from "../../core/dom.ts";
import { isRecord } from "../../core/guards.ts";
import { dispatchEditorEvent, type DraftValues } from "./events.ts";
import type { EditorMode } from "./preview.ts";

const editorModeStorageKey = "kumbuka.editor.mode";
const draftPrefix = "kumbuka.editor.draft:";

type DraftSource = "local" | "server";
type DraftState = "idle" | "pending" | "saving" | "saved" | "local";

type DraftValue = {
  pageID: number;
  title: string;
  slug: string;
  values: DraftValues;
};

type DraftCandidate = DraftValue & {
  savedAt: number;
  stale?: boolean;
  source: DraftSource;
};

type NavigationOptions = { discardDraft?: boolean; keepDraft?: boolean };

function namedControl(
  form: HTMLFormElement,
  name: string,
): HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement | null {
  const item = form.elements.namedItem(name);
  return item instanceof HTMLInputElement ||
    item instanceof HTMLTextAreaElement ||
    item instanceof HTMLSelectElement
    ? item
    : null;
}

function isDraftValues(value: unknown): value is DraftValues {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return false;
  return Object.values(value).every(
    (entry) =>
      Array.isArray(entry) && entry.every((item) => typeof item === "string"),
  );
}

export { slugifyEditorPath };

export function resolvedGuidedEditorPath(
  title: string,
  parent: string,
  fixedSegment = "",
): string {
  const normalizedParent = parent.trim().replace(/^\/+|\/+$/g, "");
  const segment = slugifyEditorPath(fixedSegment || title).replaceAll("/", "-");

  return [normalizedParent, segment].filter(Boolean).join("/");
}

// Calculates word, character, and line counts.
export function editorWordStats(value: string): {
  words: number;
  characters: number;
  lines: number;
} {
  const trimmed = value.trim();
  return {
    words: trimmed ? trimmed.split(/\s+/u).length : 0,
    characters: value.length,
    lines: value ? value.split("\n").length : 1,
  };
}

function formSnapshot(form: HTMLFormElement): string {
  const values: [string, string][] = [];

  for (const [name, value] of new FormData(form).entries())
    values.push([name, String(value)]);

  values.sort(([leftName, leftValue], [rightName, rightValue]) =>
    leftName === rightName
      ? leftValue.localeCompare(rightValue)
      : leftName.localeCompare(rightName),
  );
  return JSON.stringify(values);
}

function draftValues(form: HTMLFormElement): DraftValues {
  const values: DraftValues = {};

  for (const [name, value] of new FormData(form).entries()) {
    values[name] ??= [];
    values[name].push(String(value));
  }

  return values;
}

function draftValue(form: HTMLFormElement): DraftValue {
  const values = draftValues(form);
  return {
    pageID: Number.parseInt(form.dataset.pageId || "0", 10) || 0,
    title: String(values.title?.[0] || ""),
    slug: String(values.slug?.[0] || ""),
    values,
  };
}

function draftKey(form: HTMLFormElement): string {
  return `${draftPrefix}${form.dataset.draftKey || "new"}`;
}

function normalizeLocalDraft(value: unknown): DraftCandidate | null {
  if (typeof value !== "object" || value === null) return null;

  const draft = value as Record<string, unknown>;
  const savedAt = Number(draft.savedAt);
  if (!Number.isFinite(savedAt) || savedAt <= 0) return null;
  if (!isDraftValues(draft.values)) return null;
  if (typeof draft.title !== "string" || typeof draft.slug !== "string")
    return null;
  const pageID = draft.pageID ?? 0;
  if (typeof pageID !== "number" || !Number.isSafeInteger(pageID) || pageID < 0)
    return null;

  return {
    pageID,
    title: draft.title,
    slug: draft.slug,
    values: draft.values,
    savedAt,
    source: "local",
  };
}

function parseStoredDraft(form: HTMLFormElement): DraftCandidate | null {
  try {
    const value = localStorage.getItem(draftKey(form));
    if (!value) return null;
    return normalizeLocalDraft(JSON.parse(value) as unknown);
  } catch {
    return null;
  }
}

// Reject malformed server state rather than coercing identifiers or field values.
export function normalizeServerDraft(value: unknown): DraftCandidate | null {
  if (!isRecord(value) || !isDraftValues(value.values)) return null;
  if (typeof value.title !== "string" || typeof value.slug !== "string")
    return null;
  if (typeof value.updated_at !== "string" || typeof value.stale !== "boolean")
    return null;

  const pageID = value.page_id ?? 0;
  if (typeof pageID !== "number" || !Number.isSafeInteger(pageID) || pageID < 0)
    return null;
  const savedAt = Date.parse(value.updated_at);
  if (!Number.isFinite(savedAt)) return null;

  return {
    pageID,
    title: value.title,
    slug: value.slug,
    values: value.values,
    savedAt,
    stale: value.stale,
    source: "server",
  };
}

function comparableDraftValues(values: DraftValues): string {
  return JSON.stringify(
    Object.keys(values)
      .sort()
      .map((name) => [name, (values[name] || []).map(String)]),
  );
}

function draftDiffers(
  candidate: DraftCandidate | null,
  current: DraftValue,
): boolean {
  return Boolean(
    candidate &&
    comparableDraftValues(candidate.values) !==
      comparableDraftValues(current.values),
  );
}

function restoreDraftValues(form: HTMLFormElement, values: DraftValues): void {
  dispatchEditorEvent(form, "editor:restore-draft", { values });

  const dynamicNames = new Set(["property_key", "property_value"]);
  const protectedNames = new Set(["original_slug"]);
  const controls = [...form.elements].filter(
    (
      element,
    ): element is HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement =>
      element instanceof HTMLInputElement ||
      element instanceof HTMLTextAreaElement ||
      element instanceof HTMLSelectElement,
  );
  const names = new Set(
    controls.map((element) => element.name).filter(Boolean),
  );

  for (const name of Object.keys(values)) names.add(name);

  for (const name of names) {
    if (dynamicNames.has(name) || protectedNames.has(name)) continue;

    const matching = controls.filter((element) => element.name === name);
    const desired = (values[name] || []).map(String);
    if (!matching.length) continue;

    let valueIndex = 0;

    for (const control of matching) {
      if (
        control instanceof HTMLInputElement &&
        (control.type === "checkbox" || control.type === "radio")
      ) {
        control.checked = desired.includes(String(control.value || "on"));
      } else if (control instanceof HTMLSelectElement && control.multiple) {
        for (const option of control.options)
          option.selected = desired.includes(String(option.value));
      } else {
        control.value = desired[valueIndex] ?? desired[0] ?? "";
        valueIndex += 1;
      }

      control.dispatchEvent(new Event("input", { bubbles: true }));
      control.dispatchEvent(new Event("change", { bubbles: true }));
    }
  }
}

async function loadServerDraft(url: string): Promise<DraftCandidate | null> {
  try {
    const response = await fetch(url, {
      headers: { Accept: "application/json" },
    });
    if (response.status === 404 || !response.ok) return null;

    return normalizeServerDraft(await response.json());
  } catch {
    return null;
  }
}

async function saveServerDraft(
  url: string,
  draft: DraftValue,
): Promise<unknown> {
  const response = await fetch(url, {
    method: "PUT",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify({
      page_id: draft.pageID,
      title: draft.title,
      slug: draft.slug,
      values: draft.values,
    }),
  });
  if (!response.ok) throw new Error("server draft save failed");

  const payload: unknown = await response.json();
  if (!normalizeServerDraft(payload))
    throw new Error("Invalid server draft response.");
  return payload;
}

async function deleteServerDraft(url: string): Promise<void> {
  const response = await fetch(url, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) throw new Error("server draft delete failed");
}

function setupDrafts(
  form: HTMLFormElement,
  markDirty: () => void,
  setDraftState: (state: DraftState) => void,
): {
  clearDraft: (deleteServer?: boolean) => Promise<void>;
  flushDraft: () => Promise<boolean>;
} {
  const banner = form.querySelector<HTMLElement>("[data-editor-draft-banner]");
  const bannerMessage = form.querySelector<HTMLElement>(
    "[data-editor-draft-message]",
  );
  const restore = form.querySelector<HTMLButtonElement>(
    "[data-editor-draft-restore]",
  );
  const discard = form.querySelector<HTMLButtonElement>(
    "[data-editor-draft-discard]",
  );
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  if (!banner || !restore || !discard || !source)
    return { clearDraft: async () => {}, flushDraft: async () => true };

  const draftURL = requiredAttribute(form, "data-draft-url");
  let timer: ReturnType<typeof setTimeout> | undefined;
  let generation = 0;
  let candidate: DraftCandidate | null = null;

  void (async () => {
    const local = parseStoredDraft(form);
    const server = await loadServerDraft(draftURL);

    candidate =
      [local, server]
        .filter((item): item is DraftCandidate => item !== null)
        .sort((left, right) => right.savedAt - left.savedAt)[0] ?? null;
    if (!draftDiffers(candidate, draftValue(form))) return;

    if (bannerMessage && candidate) {
      bannerMessage.textContent = candidate.stale
        ? "A private draft is available, but the page changed after this draft was started."
        : candidate.source === "server"
          ? "A newer private server draft is available."
          : "A newer browser fallback draft is available.";
    }

    banner.hidden = false;
  })();

  restore.addEventListener("click", () => {
    if (!candidate) return;

    restoreDraftValues(form, candidate.values);
    banner.hidden = true;
    markDirty();
    setDraftState(candidate.source === "server" ? "saved" : "local");
    source.focus();
  });

  const clearDraft = async (deleteServer = true): Promise<void> => {
    generation += 1;

    if (timer) clearTimeout(timer);
    try {
      localStorage.removeItem(draftKey(form));
    } catch {
      // Browser fallback cleanup is best effort.
    }
    if (deleteServer) await deleteServerDraft(draftURL);
  };

  discard.addEventListener("click", async () => {
    try {
      await clearDraft(true);
      candidate = null;
      banner.hidden = true;
    } catch {
      await showNotice("The private server draft could not be discarded.", {
        title: "Discard failed",
      });
    }
  });

  const persistLocalDraft = (): DraftValue => {
    const value = draftValue(form);

    try {
      localStorage.setItem(
        draftKey(form),
        JSON.stringify({ ...value, savedAt: Date.now() }),
      );
    } catch {
      /* best effort */
    }

    return value;
  };

  const persistServerDraft = async (
    value: DraftValue,
    currentGeneration: number,
  ): Promise<boolean> => {
    setDraftState("saving");
    try {
      await saveServerDraft(draftURL, value);

      if (currentGeneration === generation) setDraftState("saved");

      return true;
    } catch {
      if (currentGeneration === generation) setDraftState("local");
      return false;
    }
  };

  const schedule = (): void => {
    if (timer) clearTimeout(timer);

    generation += 1;

    const currentGeneration = generation;
    const value = persistLocalDraft();

    setDraftState("pending");
    timer = setTimeout(
      () => void persistServerDraft(value, currentGeneration),
      650,
    );
  };

  const flushDraft = async (): Promise<boolean> => {
    if (timer) clearTimeout(timer);

    generation += 1;

    const currentGeneration = generation;

    return persistServerDraft(persistLocalDraft(), currentGeneration);
  };

  form.addEventListener("input", schedule);
  form.addEventListener("change", schedule);
  return { clearDraft, flushDraft };
}

// The shared picker owns parent selection; the editor derives the page slug.
function setupPathPreview(form: HTMLFormElement): void {
  const title = namedControl(form, "title");
  const slug = namedControl(form, "slug");
  const parent = namedControl(form, "parent_path");
  const preview = form.querySelector<HTMLElement>("[data-editor-path-preview]");
  const chromeTitle = form.querySelector<HTMLElement>(
    "[data-editor-chrome-title]",
  );
  if (!title || !slug || !parent || !preview) return;
  const fixedSegment = slug.dataset.editorPathSegment || "";
  const update = (): void => {
    const path = resolvedGuidedEditorPath(
      title.value,
      parent.value,
      fixedSegment,
    );
    slug.value = path;
    preview.textContent = path
      ? `Pages / ${path.split("/").join(" / ")}`
      : "Pages / new-page";
    if (chromeTitle) chromeTitle.textContent = title.value.trim() || "Untitled";
  };
  title.addEventListener("input", update);
  parent.addEventListener("input", update);
  parent.addEventListener("change", update);
  form.addEventListener("editor:restore-draft", () =>
    requestAnimationFrame(update),
  );
  update();
}

function setupStats(form: HTMLFormElement): void {
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const words = form.querySelector<HTMLElement>("[data-editor-word-count]");
  const characters = form.querySelector<HTMLElement>(
    "[data-editor-character-count]",
  );
  const lines = form.querySelector<HTMLElement>("[data-editor-line-count]");
  if (!source || !words || !characters || !lines) return;

  const editor = source;
  const wordCount = words;
  const characterCount = characters;
  const lineCount = lines;

  function update(): void {
    const stats = editorWordStats(editor.value);

    wordCount.textContent = `${stats.words} word${stats.words === 1 ? "" : "s"}`;
    characterCount.textContent = `${stats.characters} character${stats.characters === 1 ? "" : "s"}`;
    lineCount.textContent = `${stats.lines} line${stats.lines === 1 ? "" : "s"}`;
  }

  editor.addEventListener("input", update);
  update();
}

function setupFocusMode(form: HTMLFormElement): void {
  const button = form.querySelector<HTMLButtonElement>(
    "[data-editor-focus-toggle]",
  );
  const label = button?.querySelector<HTMLElement>("[data-editor-focus-label]");
  if (!button || !label) return;

  const focusButton = button;
  const focusLabel = label;

  function setFocusMode(active: boolean): void {
    document.body.classList.toggle("editor-focus-mode", active);
    form.classList.toggle("focus-mode", active);
    focusButton.classList.toggle("active", active);
    focusButton.setAttribute("aria-pressed", String(active));
    focusLabel.textContent = active ? "Exit focus" : "Focus";
  }

  focusButton.addEventListener("click", () =>
    setFocusMode(!form.classList.contains("focus-mode")),
  );
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key === "Escape" && form.classList.contains("focus-mode"))
      setFocusMode(false);
  });
}

function isSameDocumentAnchor(link: HTMLAnchorElement): boolean {
  try {
    const current = new URL(window.location.href);
    const target = new URL(link.href, current);

    return (
      target.origin === current.origin &&
      target.pathname === current.pathname &&
      target.search === current.search &&
      target.hash !== ""
    );
  } catch {
    return false;
  }
}

async function confirmLeaveEditor(): Promise<boolean> {
  return requestConfirmation(
    "This page is not published yet. Your private draft will be kept so you can continue later.",
    {
      eyebrow: "Private draft",
      title: "Leave editor?",
      confirmLabel: "Keep draft and leave",
      cancelLabel: "Keep editing",
      danger: false,
    },
  );
}

async function confirmDiscardChanges(): Promise<boolean> {
  return requestConfirmation(
    "Discard all unsaved changes and leave the editor? This cannot be undone.",
    {
      eyebrow: "Unsaved changes",
      title: "Discard changes?",
      confirmLabel: "Discard changes",
      cancelLabel: "Keep editing",
      danger: true,
    },
  );
}

function setupEditorExperience(form: HTMLFormElement): void {
  const status = form.querySelector<HTMLElement>("[data-editor-save-status]");
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const discardButton = form.querySelector<HTMLButtonElement>(
    "[data-editor-discard]",
  );
  if (!status || !source) return;

  const saveStatus = status;

  let submitting = false;
  let initial = formSnapshot(form);
  let dirty = false;
  let draftState: DraftState = "idle";
  const pristineLabel = form.dataset.editorNew === "true" ? "Ready" : "Saved";
  const exitURL = requiredAttribute(form, "data-editor-exit-url");

  function renderDirty(): void {
    saveStatus.classList.toggle("dirty", dirty);
    saveStatus.classList.toggle("saved", !dirty || draftState === "saved");
    if (!dirty) {
      saveStatus.textContent = pristineLabel;
      return;
    }

    switch (draftState) {
      case "saving":
        saveStatus.textContent = "Saving draft…";
        break;
      case "saved":
        saveStatus.textContent = "Draft saved";
        break;
      case "local":
        saveStatus.textContent = "Draft saved locally";
        break;
      default:
        saveStatus.textContent = "Unsaved changes";
    }
  }

  function updateDirty(): void {
    dirty = formSnapshot(form) !== initial;
    renderDirty();
  }
  function setDraftState(state: DraftState): void {
    draftState = state;
    renderDirty();
  }

  function shouldConfirmNavigation(event: MouseEvent): boolean {
    if (submitting || !dirty) return false;
    if (event.defaultPrevented || event.button !== 0) return false;
    const modified =
      event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
    if (modified) return false;

    return true;
  }

  const { clearDraft, flushDraft } = setupDrafts(
    form,
    updateDirty,
    setDraftState,
  );

  setupPathPreview(form);
  setupStats(form);
  setupFocusMode(form);

  async function navigate(
    url: string,
    { discardDraft = false, keepDraft = false }: NavigationOptions = {},
  ): Promise<void> {
    if (discardDraft) {
      try {
        await clearDraft(true);
      } catch {
        await showNotice("The private server draft could not be discarded.", {
          title: "Discard failed",
        });
        return;
      }
    } else if (keepDraft) await flushDraft();

    submitting = true;
    window.location.assign(url);
  }

  function markSubmitted(): void {
    submitting = true;
    dirty = false;
    void clearDraft(false);
    initial = formSnapshot(form);
    renderDirty();
  }

  form.addEventListener("input", updateDirty);
  form.addEventListener("change", updateDirty);
  form.addEventListener("submit", (event) => {
    if (event.defaultPrevented) return;
    markSubmitted();
  });
  form.addEventListener("kumbuka:form-submit-success", markSubmitted);

  discardButton?.addEventListener("click", async () => {
    if (dirty && !(await confirmDiscardChanges())) return;
    await navigate(exitURL, { discardDraft: true });
  });

  document.addEventListener("click", async (event: MouseEvent) => {
    if (!shouldConfirmNavigation(event)) return;

    const targetNode = event.target;
    if (!(targetNode instanceof Element)) return;

    const link = targetNode.closest<HTMLAnchorElement>("a[href]");
    if (!link || link.hasAttribute("download")) return;

    const target = link.getAttribute("target");
    if (target && target !== "_self") return;
    if (isSameDocumentAnchor(link)) return;

    event.preventDefault();
    if (!(await confirmLeaveEditor())) return;

    await navigate(link.href, { keepDraft: true });
  });

  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (!(event.ctrlKey || event.metaKey)) return;

    const key = event.key.toLocaleLowerCase();

    switch (key) {
      case "s":
        event.preventDefault();
        form.requestSubmit();
        break;
      case "p":
        if (event.shiftKey) {
          event.preventDefault();
          dispatchEditorEvent(form, "editor:toggle-preview");
        }
        break;
    }
  });
  renderDirty();
}

export function preferredEditorMode(): EditorMode {
  try {
    return localStorage.getItem(editorModeStorageKey) === "visual"
      ? "visual"
      : "write";
  } catch {
    return "write";
  }
}

export function rememberEditorMode(mode: EditorMode): void {
  // Split preview is a temporary Markdown aid, not a primary editor mode.
  if (mode === "split") return;

  try {
    localStorage.setItem(editorModeStorageKey, mode);
  } catch {
    /* best effort */
  }
}

export function initEditorExperience(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupEditorExperience(form);
}
