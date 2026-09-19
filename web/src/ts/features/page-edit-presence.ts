// Lightweight page-edit presence for viewers and concurrent editors.

import { isRecord } from "../core/guards.ts";

const viewerPollMilliseconds = 15_000;
const editorHeartbeatMilliseconds = 20_000;

type PageEditorPresence = {
  userID: number;
  name: string;
};

function pageSlug(): { slug: string; editing: boolean } | null {
  const editor = document.querySelector<HTMLFormElement>(
    "form[data-editor-form]",
  );
  if (editor) {
    const control = editor.elements.namedItem("original_slug");
    if (control instanceof HTMLInputElement && control.value.trim()) {
      return { slug: control.value.trim(), editing: true };
    }
    return null;
  }

  if (!document.querySelector<HTMLElement>("[data-page-layout]")) return null;
  const prefix = "/pages/";
  if (!window.location.pathname.startsWith(prefix)) return null;

  const encoded = window.location.pathname.slice(prefix.length);
  if (!encoded) return null;

  try {
    return { slug: decodeURIComponent(encoded), editing: false };
  } catch {
    return null;
  }
}

function presenceURL(slug: string): string {
  const path = slug
    .split("/")
    .map((segment) => encodeURIComponent(segment))
    .join("/");
  return `/api/page-presence/${path}`;
}

function parseEditors(value: unknown): PageEditorPresence[] {
  if (!isRecord(value) || !Array.isArray(value.editors)) return [];

  return value.editors.flatMap((candidate): PageEditorPresence[] => {
    if (!isRecord(candidate)) return [];
    if (
      typeof candidate.user_id !== "number" ||
      !Number.isSafeInteger(candidate.user_id) ||
      candidate.user_id <= 0 ||
      typeof candidate.name !== "string" ||
      !candidate.name.trim()
    ) {
      return [];
    }

    return [{ userID: candidate.user_id, name: candidate.name.trim() }];
  });
}

function editorNames(editors: PageEditorPresence[]): string {
  const names = editors.map((editor) => editor.name);
  if (names.length <= 2) return names.join(" and ");
  if (names.length === 3) return `${names[0]}, ${names[1]} and ${names[2]}`;
  return `${names[0]}, ${names[1]} and ${names.length - 2} others`;
}

function createBanner(editing: boolean): HTMLElement | null {
  const host = editing
    ? document.querySelector<HTMLFormElement>("form[data-editor-form]")
    : document.querySelector<HTMLElement>("[data-page-layout]");
  if (!host) return null;

  const banner = document.createElement("div");
  banner.className = `page-edit-presence${editing ? " editor-presence" : ""}`;
  banner.dataset.pageEditPresence = "true";
  banner.setAttribute("role", "status");
  banner.setAttribute("aria-live", "polite");
  banner.hidden = true;

  const dot = document.createElement("span");
  dot.className = "page-edit-presence-dot";
  dot.setAttribute("aria-hidden", "true");
  const message = document.createElement("span");
  message.dataset.pageEditPresenceMessage = "true";
  banner.append(dot, message);

  if (editing) {
    const chrome = host.querySelector<HTMLElement>(".editor-chrome");
    if (chrome) chrome.insertAdjacentElement("afterend", banner);
    else host.prepend(banner);
  } else {
    const heading = host.querySelector<HTMLElement>(".page-heading");
    if (heading) heading.insertAdjacentElement("afterend", banner);
    else host.prepend(banner);
  }

  return banner;
}

function renderPresence(
  banner: HTMLElement,
  editors: PageEditorPresence[],
  editing: boolean,
): void {
  const message = banner.querySelector<HTMLElement>(
    "[data-page-edit-presence-message]",
  );
  if (!message) return;

  if (editors.length === 0) {
    banner.hidden = true;
    message.textContent = "";
    return;
  }

  const names = editorNames(editors);
  const verb = editors.length === 1 ? "is" : "are";
  message.textContent = editing
    ? `${names} ${verb} also editing this page. You can continue editing; Kumbuka will prevent a stale save from overwriting newer changes.`
    : `${names} ${verb} editing this page.`;
  banner.hidden = false;
}

async function fetchEditors(url: string): Promise<PageEditorPresence[] | null> {
  try {
    const response = await fetch(url, {
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
    if (!response.ok) return null;
    return parseEditors(await response.json());
  } catch {
    return null;
  }
}

async function touchEditor(url: string): Promise<void> {
  try {
    await fetch(url, {
      method: "PUT",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });
  } catch {
    // Presence is advisory; editing must keep working while it is unavailable.
  }
}

function leaveEditor(url: string): void {
  void fetch(url, {
    method: "DELETE",
    credentials: "same-origin",
    headers: { Accept: "application/json" },
    keepalive: true,
  }).catch(() => {});
}

// initPageEditPresence shows active editors on both page and editor surfaces.
export function initPageEditPresence(): void {
  const page = pageSlug();
  if (!page) return;

  const banner = createBanner(page.editing);
  if (!banner) return;

  const url = presenceURL(page.slug);
  let active = true;

  const refresh = async (): Promise<void> => {
    if (!active) return;
    if (page.editing) await touchEditor(url);
    if (!active) return;
    const editors = await fetchEditors(url);
    if (editors) renderPresence(banner, editors, page.editing);
  };

  void refresh();
  const interval = window.setInterval(
    () => void refresh(),
    page.editing ? editorHeartbeatMilliseconds : viewerPollMilliseconds,
  );

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState === "visible") void refresh();
  });

  window.addEventListener("pagehide", () => {
    active = false;
    window.clearInterval(interval);
    if (page.editing) leaveEditor(url);
  });
}
