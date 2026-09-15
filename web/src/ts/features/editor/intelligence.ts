import { slugifyPagePath as editorSlug } from "../../core/slug.ts";
// Editor diagnostics, outline, and metadata assistance.

import { requestJSON } from "../../core/http.ts";
import { parseEditorCatalog, type EditorCatalog } from "./catalog.ts";
import type { DraftValues } from "./events.ts";

const catalogURL = "/api/editor/catalog";

type MarkdownHeading = { level: number; title: string; offset: number };
type SourceRange = { start: number; end: number };
type WikiLinkRange = SourceRange & { target: string; raw: string };
type DiagnosticKind = "error" | "warning" | "suggestion";
type Diagnostic = SourceRange & {
  kind: DiagnosticKind;
  code: string;
  title: string;
  detail?: string;
  replacement?: string;
};

export { editorSlug };

// Extracts Markdown headings while ignoring fenced code.
export function markdownHeadings(source: string): MarkdownHeading[] {
  const headings: MarkdownHeading[] = [];
  let offset = 0;
  let fence = "";

  for (const line of String(source || "").split("\n")) {
    const trimmed = line.trimStart();

    if (!fence && (/^```/.test(trimmed) || /^~~~/.test(trimmed)))
      fence = trimmed.slice(0, 3);
    else if (fence && trimmed.startsWith(fence)) fence = "";
    else if (!fence) {
      const match = line.match(/^\s{0,3}(#{1,6})\s+(.+?)\s*#*\s*$/u);
      if (match)
        headings.push({
          level: match[1].length,
          title: match[2].trim(),
          offset,
        });
    }

    offset += line.length + 1;
  }

  return headings;
}

// Returns wiki-link ranges and targets in Markdown.
export function wikiLinkRanges(source: string): WikiLinkRange[] {
  const ranges: WikiLinkRange[] = [];
  const pattern = /\[\[([^\]|]+)(?:\|[^\]]+)?\]\]/gu;

  for (const match of String(source || "").matchAll(pattern)) {
    ranges.push({
      start: match.index,
      end: match.index + match[0].length,
      target: match[1].trim(),
      raw: match[0],
    });
  }

  return ranges;
}

function inRanges(index: number, ranges: SourceRange[]): boolean {
  return ranges.some((range) => index >= range.start && index < range.end);
}

function lineNumberAt(source: string, offset: number): number {
  return source.slice(0, offset).split("\n").length;
}

function protectedRanges(source: string): SourceRange[] {
  const ranges: SourceRange[] = wikiLinkRanges(source);
  const fencePattern =
    /(^|\n)\s{0,3}(```|~~~)[^\n]*\n[\s\S]*?\n\s{0,3}\2(?=\n|$)/gu;

  for (const match of source.matchAll(fencePattern))
    ranges.push({ start: match.index, end: match.index + match[0].length });

  const inlineCode = /`[^`\n]+`/gu;

  for (const match of source.matchAll(inlineCode))
    ranges.push({ start: match.index, end: match.index + match[0].length });

  return ranges;
}

// Builds editor diagnostics from Markdown and catalog data.
export function editorDiagnostics(
  source: string,
  catalogValue: unknown,
  currentSlug = "",
): Diagnostic[] {
  const diagnostics: Diagnostic[] = [];
  const catalog = parseEditorCatalog(catalogValue);
  const { pages, aliases } = catalog;
  const pageBySlug = new Map(
    pages.map((page) => [editorSlug(page.slug), page]),
  );
  const links = wikiLinkRanges(source);

  for (const link of links) {
    const slug = editorSlug(link.target);
    if (!pageBySlug.has(slug) && !Object.hasOwn(aliases, slug)) {
      diagnostics.push({
        kind: "error",
        code: "broken-link",
        title: `Missing page: ${link.target}`,
        detail: `Line ${lineNumberAt(source, link.start)} · ${link.raw}`,
        start: link.start,
        end: link.end,
      });
    }
  }

  const headings = markdownHeadings(source);

  for (let index = 1; index < headings.length; index += 1) {
    const previous = headings[index - 1];
    const current = headings[index];

    if (current.level > previous.level + 1) {
      diagnostics.push({
        kind: "warning",
        code: "heading-jump",
        title: `Heading jumps from H${previous.level} to H${current.level}`,
        detail: `“${current.title}”`,
        start: current.offset,
        end: current.offset + current.title.length + current.level + 1,
      });
    }
  }

  const protectedContent = protectedRanges(source);
  const normalizedCurrent = editorSlug(currentSlug);
  const lowered = source.toLocaleLowerCase();
  const candidates = pages
    .filter(
      (page) =>
        editorSlug(page.slug) !== normalizedCurrent &&
        page.title.trim().length >= 4,
    )
    .sort((left, right) => right.title.length - left.title.length);
  let suggestions = 0;

  for (const page of candidates) {
    if (suggestions >= 5) break;

    const title = page.title.trim();
    const needle = title.toLocaleLowerCase();
    let start = lowered.indexOf(needle);

    while (start >= 0 && inRanges(start, protectedContent))
      start = lowered.indexOf(needle, start + needle.length);

    if (start < 0) continue;

    const before = start === 0 ? " " : source[start - 1];
    const after =
      start + title.length >= source.length
        ? " "
        : source[start + title.length];
    if (/\p{L}|\p{N}/u.test(before) || /\p{L}|\p{N}/u.test(after)) continue;

    diagnostics.push({
      kind: "suggestion",
      code: "link-suggestion",
      title: `Link “${source.slice(start, start + title.length)}”`,
      detail: page.slug,
      start,
      end: start + title.length,
      replacement: `[[${title}]]`,
    });
    suggestions += 1;
  }

  return diagnostics;
}

function createInlineSummary(
  source: HTMLTextAreaElement,
  diagnostics: Diagnostic[],
): void {
  const pane = source.closest<HTMLElement>(".editor-source-pane");
  const existing = pane?.querySelector<HTMLElement>(
    "[data-editor-inline-diagnostics]",
  );
  const bar = existing || document.createElement("div");

  bar.className = "editor-inline-diagnostics";
  bar.dataset.editorInlineDiagnostics = "";

  if (!existing) pane?.append(bar);

  const errors = diagnostics.filter((item) => item.kind === "error").length;
  const warnings = diagnostics.filter((item) => item.kind === "warning").length;
  const suggestions = diagnostics.filter(
    (item) => item.kind === "suggestion",
  ).length;

  bar.replaceChildren();
  if (!errors && !warnings && !suggestions) {
    bar.hidden = true;
    return;
  }

  bar.hidden = false;

  const summaries: [string, number, DiagnosticKind][] = [
    ["problem", errors, "error"],
    ["warning", warnings, "warning"],
    ["link suggestion", suggestions, "suggestion"],
  ];

  for (const [label, count, kind] of summaries) {
    if (!count) continue;

    const button = document.createElement("button");

    button.type = "button";
    button.dataset.inlineDiagnosticKind = kind;
    button.textContent = `${count} ${label}${count === 1 ? "" : "s"}`;
    bar.append(button);
  }
}

function setupProperties(form: HTMLFormElement): void {
  const list = form.querySelector<HTMLElement>("[data-page-property-list]");
  const add = form.querySelector<HTMLButtonElement>("[data-page-property-add]");
  if (!list || !add) return;

  const propertyList = list;

  const makeRow = (): HTMLDivElement => {
    const row = document.createElement("div");

    row.className = "editor-property-row";
    row.innerHTML =
      '<input name="property_key" placeholder="Environment"><input name="property_value" placeholder="Production"><button class="icon-button" type="button" data-page-property-remove aria-label="Remove property">×</button>';
    return row;
  };

  add.addEventListener("click", () => {
    const row = makeRow();

    propertyList.append(row);
    row.querySelector<HTMLInputElement>("input")?.focus();
    form.dispatchEvent(new Event("change", { bubbles: true }));
  });
  propertyList.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    target
      .closest("[data-page-property-remove]")
      ?.closest(".editor-property-row")
      ?.remove();
    form.dispatchEvent(new Event("change", { bubbles: true }));
  });

  form.addEventListener("editor:restore-draft", (event) => {
    const values: DraftValues = event.detail.values;
    const keys = Array.isArray(values.property_key) ? values.property_key : [];
    const propertyValues = Array.isArray(values.property_value)
      ? values.property_value
      : [];

    propertyList.replaceChildren();

    for (
      let index = 0;
      index < Math.max(keys.length, propertyValues.length);
      index += 1
    ) {
      const row = makeRow();
      const inputs = row.querySelectorAll<HTMLInputElement>("input");

      if (inputs[0]) inputs[0].value = String(keys[index] || "");
      if (inputs[1]) inputs[1].value = String(propertyValues[index] || "");

      propertyList.append(row);
    }
  });

  const status = form.querySelector<HTMLSelectElement>("[data-page-status]");
  const target = form.querySelector<HTMLElement>("[data-deprecated-target]");
  const updateStatus = (): void => {
    if (target) target.hidden = status?.value !== "deprecated";
  };

  status?.addEventListener("change", updateStatus);
  updateStatus();
}

function setupIntelligence(form: HTMLFormElement): void {
  const source = form.querySelector<HTMLTextAreaElement>(
    "[data-markdown-editor]",
  );
  const inspector = form.querySelector<HTMLElement>("[data-editor-inspector]");
  const problems = form.querySelector<HTMLElement>("[data-editor-problems]");
  const outline = form.querySelector<HTMLElement>("[data-editor-outline]");
  const count = form.querySelector<HTMLElement>("[data-editor-problem-count]");
  if (!source || !inspector || !problems || !outline || !count) return;

  const editor = source;
  const inspectorPanel = inspector;
  const problemPanel = problems;
  const outlinePanel = outline;
  const problemCount = count;

  let catalog: EditorCatalog = {
    pages: [],
    aliases: {},
    completions: [],
    inserts: [],
  };
  let diagnostics: Diagnostic[] = [];
  let activeTab: "problems" | "outline" = "problems";
  let timer: ReturnType<typeof setTimeout> | undefined;

  function focusRange(start: number, end: number): void {
    editor.focus();
    editor.setSelectionRange(start, end);

    const line = editor.value.slice(0, start).split("\n").length - 1;
    const lineHeight =
      Number.parseFloat(getComputedStyle(editor).lineHeight) || 22;

    editor.scrollTop = Math.max(0, line * lineHeight - editor.clientHeight / 3);
  }

  function renderProblems(): void {
    problemPanel.replaceChildren();
    problemCount.textContent = String(
      diagnostics.filter((item) => item.kind !== "suggestion").length,
    );
    createInlineSummary(editor, diagnostics);
    if (!diagnostics.length) {
      const empty = document.createElement("div");

      empty.className = "editor-inspector-empty";
      empty.textContent = "No problems found.";
      problemPanel.append(empty);
      return;
    }

    for (const item of diagnostics) {
      const row = document.createElement("article");

      row.className = `editor-diagnostic ${item.kind}`;

      const text = document.createElement("button");

      text.type = "button";
      text.className = "editor-diagnostic-main";
      text.innerHTML = "<strong></strong><small></small>";

      const strong = text.querySelector<HTMLElement>("strong");
      const small = text.querySelector<HTMLElement>("small");

      if (strong) strong.textContent = item.title;
      if (small) small.textContent = item.detail || "";

      text.addEventListener("click", () => focusRange(item.start, item.end));
      row.append(text);

      if (item.replacement) {
        const fix = document.createElement("button");

        fix.type = "button";
        fix.className = "button compact";
        fix.textContent = "Link";
        fix.addEventListener("click", () => {
          editor.setRangeText(
            item.replacement ?? "",
            item.start,
            item.end,
            "end",
          );
          editor.dispatchEvent(new Event("input", { bubbles: true }));
          editor.focus();
        });
        row.append(fix);
      }

      problemPanel.append(row);
    }
  }

  function renderOutline(): void {
    outlinePanel.replaceChildren();

    const headings = markdownHeadings(editor.value);
    if (!headings.length) {
      const empty = document.createElement("div");

      empty.className = "editor-inspector-empty";
      empty.textContent = "Add headings to build an outline.";
      outlinePanel.append(empty);
      return;
    }

    headings.forEach((heading) => {
      const button = document.createElement("button");

      button.type = "button";
      button.className = "editor-outline-item";
      button.style.setProperty("--outline-level", String(heading.level));
      button.innerHTML = `<span>H${heading.level}</span><strong></strong>`;

      const title = button.querySelector<HTMLElement>("strong");

      if (title) title.textContent = heading.title;

      button.addEventListener("click", () =>
        focusRange(
          heading.offset,
          heading.offset + heading.title.length + heading.level + 1,
        ),
      );
      outlinePanel.append(button);
    });
  }

  function render(): void {
    diagnostics = editorDiagnostics(
      editor.value,
      catalog,
      document.body.dataset.currentPage || "",
    );
    renderProblems();
    renderOutline();
  }

  function schedule(): void {
    if (timer) clearTimeout(timer);
    timer = setTimeout(render, 250);
  }

  function selectTab(tab: string | undefined, open = true): void {
    if (tab !== "problems" && tab !== "outline") return;

    activeTab = tab;

    if (open) inspectorPanel.hidden = false;

    problemPanel.hidden = tab !== "problems";
    outlinePanel.hidden = tab !== "outline";

    for (const button of form.querySelectorAll<HTMLElement>(
      "[data-editor-inspector-tab]",
    )) {
      const selected = button.dataset.editorInspectorTab === tab;

      button.classList.toggle("active", selected);
      button.setAttribute("aria-selected", String(selected));
    }
  }

  for (const button of form.querySelectorAll<HTMLElement>(
    "[data-editor-inspector-toggle]",
  )) {
    button.addEventListener("click", () =>
      selectTab(button.dataset.editorInspectorToggle),
    );
  }
  for (const button of form.querySelectorAll<HTMLElement>(
    "[data-editor-inspector-tab]",
  )) {
    button.addEventListener("click", () =>
      selectTab(button.dataset.editorInspectorTab),
    );
  }

  form
    .querySelector<HTMLElement>("[data-editor-inspector-close]")
    ?.addEventListener("click", () => {
      inspectorPanel.hidden = true;
    });
  editor.addEventListener("input", schedule);
  editor
    .closest<HTMLElement>(".editor-source-pane")
    ?.addEventListener("click", (event) => {
      const target = event.target;
      if (
        !(target instanceof Element) ||
        !target.closest("[data-inline-diagnostic-kind]")
      )
        return;

      selectTab("problems");
    });

  async function loadCatalog(url: string): Promise<void> {
    try {
      catalog = parseEditorCatalog(await requestJSON(url));
    } catch {
      // The editor remains useful without the optional catalog.
    }

    render();
  }

  void loadCatalog(catalogURL);

  selectTab(activeTab, false);
  render();
}

// Initializes editor intelligence.
export function initEditorIntelligence(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  )) {
    setupProperties(form);
    setupIntelligence(form);
  }
}
