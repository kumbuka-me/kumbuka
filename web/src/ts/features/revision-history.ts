// Shared, accessible revision comparison controls for page and audit dialogs.

import { initConfirmForms } from "../core/dialogs.ts";
import { requiredElement } from "../core/dom.ts";

// activateRevisionList wires the expansion controls of a server-rendered history fragment.
export function activateRevisionList(
  root: ParentNode,
  selectedNumber = 0,
): void {
  const list = root.querySelector<HTMLElement>("[data-revision-list]");
  if (!list) return;

  const entries = [
    ...list.querySelectorAll<HTMLElement>("[data-revision-entry]"),
  ];
  const toggleAll = list.querySelector<HTMLButtonElement>(
    "[data-revision-toggle-all]",
  );
  const toggleAllLabel = toggleAll?.querySelector<HTMLElement>("span");

  function setExpanded(entry: HTMLElement, expanded: boolean): void {
    const toggle = requiredElement<HTMLButtonElement>(
      entry,
      "[data-revision-toggle]",
    );
    const content = requiredElement<HTMLElement>(
      entry,
      "[data-revision-content]",
    );
    const number = entry.dataset.revisionNumber;

    entry.classList.toggle("is-open", expanded);
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.setAttribute(
      "aria-label",
      `${expanded ? "Hide" : "Show"} changes for revision ${number}`,
    );
    toggle.title = `${expanded ? "Hide" : "Show"} changes`;
    content.hidden = !expanded;
  }

  function refreshToggleAll(): void {
    if (!toggleAll) return;
    const expanded =
      entries.length > 0 &&
      entries.every((entry) => entry.classList.contains("is-open"));
    toggleAll.setAttribute("aria-expanded", String(expanded));
    if (toggleAllLabel)
      toggleAllLabel.textContent = expanded
        ? "Collapse all changes"
        : "Show all changes";
  }

  for (const entry of entries) {
    const toggle = requiredElement<HTMLButtonElement>(
      entry,
      "[data-revision-toggle]",
    );
    toggle.addEventListener("click", () => {
      setExpanded(entry, !entry.classList.contains("is-open"));
      refreshToggleAll();
    });
  }

  toggleAll?.addEventListener("click", () => {
    const expand = entries.some(
      (entry) => !entry.classList.contains("is-open"),
    );
    for (const entry of entries) setExpanded(entry, expand);
    refreshToggleAll();
  });

  if (selectedNumber > 0) {
    const selected = entries.find(
      (entry) => entry.dataset.revisionNumber === String(selectedNumber),
    );
    if (selected) {
      for (const entry of entries) {
        entry.classList.toggle("is-selected", entry === selected);
        setExpanded(entry, entry === selected);
      }
      selected.scrollIntoView({ block: "nearest" });
    }
  }
  refreshToggleAll();
  initConfirmForms(list);
}

// loadRevisionHistory fetches an authorized history fragment and activates its controls.
export async function loadRevisionHistory(
  body: HTMLElement,
  url: string,
  selectedNumber = 0,
  signal?: AbortSignal,
): Promise<void> {
  body.innerHTML = '<p class="muted">Loading revision history…</p>';
  try {
    const response = await fetch(url, {
      headers: { Accept: "text/html" },
      cache: "no-store",
      signal,
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);

    const content = await response.text();
    if (signal?.aborted) return;
    const fragment = document.createElement("template");
    fragment.innerHTML = content;
    if (!fragment.content.querySelector("[data-revision-list]"))
      throw new Error("Expected a revision history fragment");

    body.replaceChildren(fragment.content);
    activateRevisionList(body, selectedNumber);
  } catch (error) {
    if (signal?.aborted) return;
    console.error("failed to load revision history", error);
    body.innerHTML =
      '<p class="revision-dialog-error">Revision history could not be loaded. Close the dialog and try again.</p>';
  }
}

// revisionNumberFromURL returns the optional revision selected by a plugin action.
export function revisionNumberFromURL(url: string): number {
  const raw = new URL(url, "https://kumbuka.invalid/").searchParams.get(
    "revision",
  );
  if (!raw || !/^[1-9]\d*$/.test(raw)) return 0;
  const number = Number(raw);
  return Number.isSafeInteger(number) ? number : 0;
}
