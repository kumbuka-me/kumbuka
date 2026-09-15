// Client-side search for filesystem-backed static Kumbuka sites.

import { isRecord, requireArrayOf } from "../core/guards.ts";
import { requestJSON } from "../core/http.ts";

interface StaticSearchEntry {
  title: string;
  url: string;
  text: string;
}

interface RankedEntry {
  entry: StaticSearchEntry;
  score: number;
}

function isStaticSearchEntry(value: unknown): value is StaticSearchEntry {
  return (
    isRecord(value) &&
    typeof value.title === "string" &&
    typeof value.url === "string" &&
    typeof value.text === "string"
  );
}

function searchIndexURL(): string {
  const base = document.body.dataset.staticBasePath || "/";
  return `${base.endsWith("/") ? base : `${base}/`}search-index.json`;
}

let indexPromise: Promise<StaticSearchEntry[]> | null = null;

async function fetchIndex(): Promise<StaticSearchEntry[]> {
  try {
    const payload = await requestJSON(searchIndexURL());

    return requireArrayOf(payload, isStaticSearchEntry, "static search index");
  } catch (error: unknown) {
    console.error("static search index could not be loaded", error);
    return [];
  }
}

async function loadIndex(): Promise<StaticSearchEntry[]> {
  indexPromise ??= fetchIndex();
  return indexPromise;
}

function normalize(value: string): string {
  return value.trim().toLocaleLowerCase();
}

function rank(entry: StaticSearchEntry, query: string): number {
  const terms = normalize(query).split(/\s+/).filter(Boolean);
  if (!terms.length) return 1;

  const title = normalize(entry.title);
  const text = normalize(entry.text);
  let score = 0;

  for (const term of terms) {
    if (title === term) score += 50;
    else if (title.startsWith(term)) score += 30;
    else if (title.includes(term)) score += 20;
    else if (text.includes(term)) score += 5;
    else return 0;
  }

  return score;
}

function searchEntries(
  entries: StaticSearchEntry[],
  query: string,
  limit = 20,
): StaticSearchEntry[] {
  return entries
    .map((entry): RankedEntry => ({ entry, score: rank(entry, query) }))
    .filter(({ score }) => score > 0)
    .sort(
      (left, right) =>
        right.score - left.score ||
        left.entry.title.localeCompare(right.entry.title),
    )
    .slice(0, limit)
    .map(({ entry }) => entry);
}

function resultLink(
  entry: StaticSearchEntry,
  className: string,
): HTMLAnchorElement {
  const link = document.createElement("a");

  link.className = className;
  link.href = entry.url;

  const title = document.createElement("strong");

  title.textContent = entry.title;

  const path = document.createElement("small");

  path.textContent = new URL(entry.url, window.location.href).pathname;
  link.append(title, path);
  return link;
}

function initSuggestions(form: HTMLFormElement): void {
  const input = form.querySelector<HTMLInputElement>('input[type="search"]');
  if (!input) return;

  const searchInput = input;
  const results = document.createElement("div");

  results.className = "search-suggestions";
  results.setAttribute("role", "listbox");
  results.hidden = true;
  form.append(results);

  let timer: ReturnType<typeof setTimeout> | undefined;
  let activeIndex = -1;
  let resultLinks: HTMLAnchorElement[] = [];

  function close(): void {
    results.hidden = true;
    searchInput.setAttribute("aria-expanded", "false");
    activeIndex = -1;
    resultLinks = [];
  }

  function select(index: number): void {
    if (!resultLinks.length) return;

    activeIndex = Math.max(0, Math.min(index, resultLinks.length - 1));
    resultLinks.forEach((link, candidateIndex) => {
      const active = candidateIndex === activeIndex;

      link.classList.toggle("active", active);
      link.setAttribute("aria-selected", String(active));
    });
    resultLinks[activeIndex]?.scrollIntoView({ block: "nearest" });
  }

  async function update(): Promise<void> {
    const query = searchInput.value.trim();
    if (!query) {
      close();
      return;
    }

    const entries = searchEntries(await loadIndex(), query, 8);

    results.replaceChildren();
    resultLinks = entries.map((entry) => {
      const link = resultLink(entry, "search-suggestion");

      link.setAttribute("role", "option");
      link.setAttribute("aria-selected", "false");
      results.append(link);
      return link;
    });

    if (!entries.length) {
      const empty = document.createElement("div");

      empty.className = "search-suggestion-empty";
      empty.textContent = "No pages found.";
      results.append(empty);
    }

    results.hidden = false;
    searchInput.setAttribute("aria-expanded", "true");
    activeIndex = -1;
  }

  function schedule(delay = 100): void {
    if (timer !== undefined) clearTimeout(timer);
    timer = setTimeout(() => void update(), delay);
  }

  searchInput.addEventListener("focus", () => schedule(0));
  searchInput.addEventListener("input", () => schedule());
  searchInput.addEventListener("keydown", (event: KeyboardEvent) => {
    switch (event.key) {
      case "ArrowDown":
        event.preventDefault();
        select(activeIndex + 1);
        break;
      case "ArrowUp":
        event.preventDefault();
        select(activeIndex <= 0 ? resultLinks.length - 1 : activeIndex - 1);
        break;
      case "Enter":
        if (activeIndex >= 0) {
          event.preventDefault();
          resultLinks[activeIndex]?.click();
        }
        break;
      case "Escape":
        close();
        searchInput.blur();
        break;
    }
  });

  document.addEventListener("pointerdown", (event: PointerEvent) => {
    if (event.target instanceof Node && !form.contains(event.target)) close();
  });
}

async function renderSearchPage(container: HTMLElement): Promise<void> {
  const query =
    new URLSearchParams(window.location.search).get("q")?.trim() ?? "";
  const title = document.querySelector<HTMLElement>(
    "[data-static-search-title]",
  );

  if (title) title.textContent = query ? `Results for “${query}”` : "All pages";

  const entries = await loadIndex();
  const results = query ? searchEntries(entries, query, 100) : entries;

  container.replaceChildren();
  if (!results.length) {
    const empty = document.createElement("div");

    empty.className = "empty";

    const strong = document.createElement("strong");

    strong.textContent = "No pages found.";

    const detail = document.createElement("p");

    detail.textContent = "Try fewer words or a different phrase.";
    empty.append(strong, detail);
    container.append(empty);
    return;
  }

  for (const entry of results) {
    const row = resultLink(entry, "page-row");
    const summary = document.createElement("p");

    summary.className = "muted";
    summary.textContent = entry.text.slice(0, 220);
    row.append(summary);
    container.append(row);
  }
}

export function initStaticSearch(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-static-search]",
  )) {
    initSuggestions(form);
  }

  const pageResults = document.querySelector<HTMLElement>(
    "[data-static-search-results]",
  );

  if (pageResults) void renderSearchPage(pageResults);
}
