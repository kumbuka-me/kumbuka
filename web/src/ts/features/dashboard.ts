// Dashboard rendering and cleanup for private server drafts and browser fallbacks.

import { requestConfirmation, showNotice } from "../core/dialogs.ts";
import { errorMessage } from "../core/http.ts";

const draftPrefix = "kumbuka.editor.draft:";

interface StorageReader {
  readonly length: number;
  key(index: number): string | null;
  getItem(key: string): string | null;
}

interface BrowserDraftValue {
  title?: unknown;
  savedAt?: unknown;
  values?: Record<string, unknown>;
}

export interface LocalDraft {
  key: string;
  title: string;
  savedAt: number;
  editURL: string;
}

function isBrowserDraftValue(value: unknown): value is BrowserDraftValue {
  if (typeof value !== "object" || value === null || Array.isArray(value))
    return false;
  const values = (value as BrowserDraftValue).values;
  return (
    typeof values === "object" && values !== null && !Array.isArray(values)
  );
}

function validDraftKey(key: string): boolean {
  if (key === "new") return true;
  if (!key.startsWith("page:")) return false;

  const id = Number(key.slice("page:".length));
  return Number.isSafeInteger(id) && id > 0 && key === `page:${id}`;
}

// Returns valid Kumbuka browser drafts ordered newest first.
export function localDrafts(
  storage: StorageReader = localStorage,
): LocalDraft[] {
  const drafts: LocalDraft[] = [];

  for (let index = 0; index < storage.length; index += 1) {
    const storageKey = storage.key(index);
    if (!storageKey?.startsWith(draftPrefix)) continue;

    try {
      const raw = storage.getItem(storageKey);
      if (raw === null) continue;

      const value: unknown = JSON.parse(raw);
      if (!isBrowserDraftValue(value) || !value.savedAt) continue;

      const savedAt = Number(value.savedAt);
      if (!Number.isFinite(savedAt) || savedAt <= 0) continue;

      const key = storageKey.slice(draftPrefix.length);
      if (!validDraftKey(key)) continue;

      drafts.push({
        key,
        title: String(value.title || "Untitled"),
        savedAt,
        editURL: localDraftEditURL(key, value),
      });
    } catch {
      // Ignore malformed browser storage.
    }
  }

  return drafts.sort((a, b) => b.savedAt - a.savedAt);
}

// Resolves the editor URL for a browser fallback draft.
function localDraftEditURL(key: string, draft: BrowserDraftValue): string {
  if (key === "new") return "/pages/new";

  const originalSlug = Array.isArray(draft.values?.original_slug)
    ? String(draft.values.original_slug[0] || "")
    : "";
  return originalSlug ? `/edit/${originalSlug}` : "/";
}

// Creates one dashboard row from the shared draft template.
function createDraftItem(
  template: HTMLTemplateElement,
  draft: LocalDraft,
): HTMLElement | null {
  const fragment = template.content.cloneNode(true) as DocumentFragment;
  const item = fragment.querySelector<HTMLElement>("[data-draft-item]");
  const link = item?.querySelector<HTMLAnchorElement>("a");
  const title = item?.querySelector<HTMLElement>("strong");
  const detail = item?.querySelector<HTMLElement>("small");
  const discard = item?.querySelector<HTMLButtonElement>(
    "[data-draft-discard]",
  );
  if (!item || !link || !title || !detail || !discard) return null;

  item.dataset.draftKey = draft.key;
  link.href = draft.editURL;
  title.textContent = draft.title;

  const age = Math.max(0, Math.round((Date.now() - draft.savedAt) / 60000));

  detail.textContent = `Browser fallback · ${age < 1 ? "just now" : `${age}m ago`}`;
  discard.dataset.draftKey = draft.key;
  discard.dataset.draftServer = "false";
  return item;
}

// Deletes one server draft through the authenticated API.
async function deleteServerDraft(key: string): Promise<void> {
  const response = await fetch(`/api/drafts/${encodeURIComponent(key)}`, {
    method: "DELETE",
    headers: { Accept: "application/json" },
  });
  if (!response.ok) {
    let message = "Draft could not be discarded.";

    try {
      const body: unknown = await response.json();
      if (
        typeof body === "object" &&
        body !== null &&
        typeof (body as { error?: unknown }).error === "string"
      ) {
        message = (body as { error: string }).error;
      }
    } catch {
      // Keep the generic message for non-JSON failures.
    }

    throw new Error(message);
  }
}

function removeLocalDraft(key: string): void {
  try {
    localStorage.removeItem(`${draftPrefix}${key}`);
  } catch {
    // Browser fallback cleanup is best effort.
  }
}

function showEmptyDraftState(container: HTMLElement): void {
  if (container.querySelector("[data-draft-item]")) return;

  const empty = document.createElement("p");

  empty.className = "muted";
  empty.dataset.homeDraftsEmpty = "";
  empty.textContent = "No private drafts.";
  container.append(empty);
}

async function handleDraftDiscard(
  container: HTMLElement,
  event: MouseEvent,
): Promise<void> {
  const target = event.target;
  if (!(target instanceof Element)) return;

  const button = target.closest<HTMLButtonElement>("[data-draft-discard]");
  if (!button) return;

  const item = button.closest<HTMLElement>("[data-draft-item]");
  const key = button.dataset.draftKey || item?.dataset.draftKey;
  if (!item || !key) return;

  const accepted = await requestConfirmation(
    "Discard this private draft? The published page will not be changed.",
    {
      eyebrow: "Private draft",
      title: "Discard draft?",
      confirmLabel: "Discard draft",
      cancelLabel: "Keep draft",
      danger: true,
    },
  );
  if (!accepted) return;

  button.disabled = true;

  try {
    if (button.dataset.draftServer === "true") await deleteServerDraft(key);

    removeLocalDraft(key);
    item.remove();
    showEmptyDraftState(container);
  } catch (error) {
    button.disabled = false;
    await showNotice(errorMessage(error) || "Draft could not be discarded.", {
      title: "Discard failed",
    });
  }
}

// Initializes dashboard draft rendering and discard actions.
export function initDashboard(): void {
  const container = document.querySelector<HTMLElement>("[data-home-drafts]");
  const template = document.querySelector<HTMLTemplateElement>(
    "[data-home-draft-template]",
  );
  if (!container || !template) return;

  const serverItems = [
    ...container.querySelectorAll<HTMLElement>(
      "[data-draft-item][data-draft-key]",
    ),
  ];
  const serverKeys = new Set(
    serverItems
      .map((item) => item.dataset.draftKey)
      .filter((key): key is string => Boolean(key)),
  );
  const serverEditURLs = new Set(
    serverItems
      .map((item) => item.dataset.draftEditUrl)
      .filter((url): url is string => Boolean(url)),
  );

  let drafts: LocalDraft[] = [];

  try {
    drafts = localDrafts().filter(
      (draft) =>
        !serverKeys.has(draft.key) && !serverEditURLs.has(draft.editURL),
    );
  } catch {
    drafts = [];
  }

  for (const draft of drafts.slice(0, Math.max(0, 6 - serverKeys.size))) {
    const item = createDraftItem(template, draft);
    if (item) container.append(item);
  }

  if (container.querySelector("[data-draft-item]")) {
    container.querySelector("[data-home-drafts-empty]")?.remove();
  }

  container.addEventListener("click", (event: MouseEvent) => {
    void handleDraftDiscard(container, event);
  });
}
