// User mention detection and autocomplete.

import { isRecord, requireArrayOf } from "../core/guards.ts";
import { requestJSON } from "../core/http.ts";
import { textareaCaretOffset } from "../core/textarea.ts";

const resultLimit = 50;
let mentionMenuSequence = 0;

type Fence = { character: string; length: number };
export type MentionTrigger = { start: number; query: string };
type MentionUser = {
  username: string;
  display_name?: string;
  role?: string;
  self?: boolean;
};

// Reports whether a caret offset is inside fenced code.
function fencedCodeAt(value: string, caret: number): boolean {
  const lines = value.slice(0, caret).split("\n");
  let fence: Fence | null = null;

  for (const line of lines) {
    const match = line.match(/^ {0,3}(`{3,}|~{3,})/u);
    if (!match) continue;

    const marker = match[1];
    const next: Fence = { character: marker[0], length: marker.length };
    if (!fence) {
      fence = next;
      continue;
    }

    if (fence.character === next.character && next.length >= fence.length)
      fence = null;
  }

  return fence !== null;
}

// Reports whether a caret offset is inside inline code.
function inlineCodeAt(value: string, caret: number): boolean {
  const lineStart = value.lastIndexOf("\n", Math.max(0, caret - 1)) + 1;
  const line = value.slice(lineStart, caret);
  let open = 0;

  for (let index = 0; index < line.length;) {
    if (line[index] !== "`" || (index > 0 && line[index - 1] === "\\")) {
      index += 1;
      continue;
    }

    let end = index + 1;

    while (line[end] === "`") end += 1;

    const length = end - index;

    if (open === 0) open = length;
    else if (length === open) open = 0;

    index = end;
  }

  return open !== 0;
}

// Finds an active user-mention trigger at the caret.
export function mentionTrigger(
  value: string,
  caret: number,
): MentionTrigger | null {
  if (fencedCodeAt(value, caret) || inlineCodeAt(value, caret)) return null;

  const lineStart = value.lastIndexOf("\n", Math.max(0, caret - 1)) + 1;
  const fragment = value.slice(lineStart, caret);
  const match = fragment.match(/(^|[\s([{>])@([A-Za-z0-9_.-]*)$/u);
  if (!match || match.index === undefined) return null;

  return {
    start: lineStart + match.index + match[1].length,
    query: match[2],
  };
}

// Builds the canonical mention text for a username.
export function mentionReplacement(username: string): string {
  return `@${username}`;
}

// Builds compact initials for a person label.
function initials(user: MentionUser): string {
  const value = (user.display_name || user.username || "?").trim();
  const parts = value.split(/\s+/u).filter(Boolean);
  if (!parts.length) return "?";
  if (parts.length === 1) return parts[0].slice(0, 2).toLocaleUpperCase();

  return `${parts[0][0]}${parts.at(-1)?.[0] ?? ""}`.toLocaleUpperCase();
}

// Appends text with the matching query highlighted.
function appendHighlighted(
  target: HTMLElement,
  value: string,
  query: string,
): void {
  if (!query) {
    target.textContent = value;
    return;
  }

  const lower = value.toLocaleLowerCase();
  const start = lower.indexOf(query.toLocaleLowerCase());
  if (start < 0) {
    target.textContent = value;
    return;
  }

  target.append(document.createTextNode(value.slice(0, start)));

  const mark = document.createElement("mark");

  mark.textContent = value.slice(start, start + query.length);
  target.append(
    mark,
    document.createTextNode(value.slice(start + query.length)),
  );
}

function isMentionUser(value: unknown): value is MentionUser {
  return (
    isRecord(value) &&
    typeof value.username === "string" &&
    (value.display_name === undefined ||
      typeof value.display_name === "string") &&
    (value.role === undefined || typeof value.role === "string") &&
    (value.self === undefined || typeof value.self === "boolean")
  );
}

function mentionUsers(value: unknown): MentionUser[] {
  return requireArrayOf(value, isMentionUser, "mention user response");
}

// Returns an optional role allow-list configured on a mention field.
function mentionRoles(source: HTMLTextAreaElement): Set<string> | null {
  const value = source.dataset.mentionRoles?.trim();
  if (!value) return null;

  const roles = value
    .split(",")
    .map((role) => role.trim())
    .filter(Boolean);

  return roles.length ? new Set(roles) : null;
}

// Wires mention autocomplete behavior.
function setupMentionAutocomplete(source: HTMLTextAreaElement): void {
  const anchor = source.parentElement;
  if (!anchor) return;

  const suggestionAnchor = anchor;

  suggestionAnchor.classList.add("mention-suggestion-anchor");

  const menu = document.createElement("div");

  menu.className = "mention-suggestion-menu";
  mentionMenuSequence += 1;
  menu.id = `mention-suggestions-${mentionMenuSequence}`;
  menu.hidden = true;
  menu.setAttribute("role", "listbox");
  menu.setAttribute("aria-label", "Mention a user");
  suggestionAnchor.append(menu);

  const allowedRoles = mentionRoles(source);
  let trigger: MentionTrigger | null = null;
  let results: MentionUser[] = [];
  let active = -1;
  let request = 0;

  function close(): void {
    request += 1;
    trigger = null;
    results = [];
    active = -1;
    menu.hidden = true;
    menu.replaceChildren();
    if (source.getAttribute("aria-controls") === menu.id) {
      source.removeAttribute("aria-controls");
      source.setAttribute("aria-expanded", "false");
    }
  }

  function position(): void {
    if (menu.hidden) return;

    const sourceRect = source.getBoundingClientRect();
    const anchorRect = suggestionAnchor.getBoundingClientRect();
    const caret = textareaCaretOffset(source, source.selectionStart ?? 0);
    const menuWidth = Math.min(
      390,
      Math.max(260, suggestionAnchor.clientWidth - 16),
    );
    const rawLeft = sourceRect.left - anchorRect.left + caret.left;
    const maxLeft = Math.max(8, suggestionAnchor.clientWidth - menuWidth - 8);

    menu.style.width = `${menuWidth}px`;
    menu.style.left = `${Math.max(8, Math.min(rawLeft, maxLeft))}px`;
    menu.style.top = `${sourceRect.top - anchorRect.top + caret.top}px`;
  }

  function render(): void {
    menu.replaceChildren();

    if (!results.length) {
      const empty = document.createElement("div");

      empty.className = "mention-suggestion-empty";
      empty.textContent = trigger?.query
        ? "No matching people."
        : "No users available.";
      menu.append(empty);
    } else {
      for (const [index, user] of results.entries()) {
        const option = document.createElement("button");

        option.type = "button";
        option.className = "mention-suggestion-option";
        option.dataset.mentionIndex = String(index);
        option.setAttribute("role", "option");
        option.setAttribute("aria-selected", String(index === active));

        const avatar = document.createElement("span");

        avatar.className = "mention-suggestion-avatar";
        avatar.textContent = initials(user);

        const text = document.createElement("span");

        text.className = "mention-suggestion-text";

        const name = document.createElement("strong");

        appendHighlighted(
          name,
          user.display_name || user.username,
          trigger?.query || "",
        );

        const meta = document.createElement("small");

        appendHighlighted(meta, `@${user.username}`, trigger?.query || "");

        if (user.role) meta.append(document.createTextNode(` · ${user.role}`));
        if (user.self) meta.append(document.createTextNode(" · you"));

        text.append(name, meta);

        option.append(avatar, text);
        menu.append(option);
      }
    }

    menu.hidden = false;
    source.setAttribute("aria-controls", menu.id);
    source.setAttribute("aria-expanded", "true");
    position();
  }

  function choose(index: number): void {
    const user = results[index];
    if (!user || !trigger) return;

    const end = source.selectionStart ?? trigger.start;

    source.setRangeText(
      mentionReplacement(user.username),
      trigger.start,
      end,
      "end",
    );
    source.dispatchEvent(new Event("input", { bubbles: true }));
    close();
    source.focus();
  }

  async function refresh(): Promise<void> {
    const next = mentionTrigger(source.value, source.selectionStart ?? 0);
    if (!next) {
      close();
      return;
    }

    trigger = next;

    const currentRequest = ++request;

    try {
      const payload = await requestJSON(
        `/api/mentions/users?q=${encodeURIComponent(next.query)}`,
      );
      if (currentRequest !== request) return;

      results = mentionUsers(payload)
        .filter((user) => !allowedRoles || allowedRoles.has(user.role ?? ""))
        .slice(0, resultLimit);
      active = results.length ? 0 : -1;
      render();
    } catch (error) {
      console.error("mention search failed", error);
      if (currentRequest === request) close();
    }
  }

  source.addEventListener("input", () => void refresh());
  source.addEventListener("click", () => void refresh());
  source.addEventListener("scroll", position);
  source.addEventListener("keydown", (event: KeyboardEvent) => {
    if (menu.hidden) return;
    switch (event.key) {
      case "ArrowDown":
        if (results.length) {
          event.preventDefault();
          active = (active + 1) % results.length;
          render();
        }
        break;
      case "ArrowUp":
        if (results.length) {
          event.preventDefault();
          active = (active - 1 + results.length) % results.length;
          render();
        }
        break;
      case "Enter":
      case "Tab":
        if (active >= 0) {
          event.preventDefault();
          choose(active);
        }
        break;
      case "Escape":
        event.preventDefault();
        close();
        break;
    }
  });

  menu.addEventListener("mousedown", (event) => event.preventDefault());
  menu.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const option = target.closest<HTMLElement>("[data-mention-index]");

    if (option) choose(Number(option.dataset.mentionIndex));
  });

  document.addEventListener("click", (event) => {
    const target = event.target;
    if (target !== source && target instanceof Node && !menu.contains(target))
      close();
  });
  window.addEventListener("resize", position);
}

// Initializes mention autocomplete.
export function initMentionAutocomplete(): void {
  for (const source of document.querySelectorAll<HTMLTextAreaElement>(
    "textarea[data-mention-autocomplete]",
  )) {
    setupMentionAutocomplete(source);
  }
}
