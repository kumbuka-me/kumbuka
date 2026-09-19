// Page discussion, inline annotation, and suggestion interactions.

import {
  requiredAttribute,
  requiredElement,
  requiredElements,
} from "../core/dom.ts";

// initPageComments wires page discussion composition and inline comment threads when present.
export function initPageComments(): void {
  const commentDialog = document.querySelector<HTMLDialogElement>(
    "[data-comment-dialog]",
  );
  if (!commentDialog) return;

  setupCommentDialog(commentDialog);

  const floatingCommentButton = document.querySelector<HTMLButtonElement>(
    "[data-floating-comment-button]",
  );
  if (floatingCommentButton) setupFloatingCommentButton(floatingCommentButton);

  setupInlineCommentThreads();
}

type PageTextSelection = {
  text: string;
  rect: DOMRect;
};

function currentPageTextSelection(): PageTextSelection | null {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || !selection.rangeCount) return null;

  const range = selection.getRangeAt(0);
  const prose = document.querySelector<HTMLElement>(".page-reading .prose");
  if (
    !prose ||
    !prose.contains(range.startContainer) ||
    !prose.contains(range.endContainer)
  )
    return null;

  const text = selection.toString().trim().slice(0, 500);
  if (!text) return null;

  const rectangles = Array.from(range.getClientRects()).filter(
    (rect) => rect.width > 0 && rect.height > 0,
  );
  const rect = rectangles.at(-1) ?? range.getBoundingClientRect();
  if (rect.width <= 0 && rect.height <= 0) return null;

  return { text, rect };
}

function selectedPageText(): string {
  return currentPageTextSelection()?.text ?? "";
}

function setupFloatingCommentButton(button: HTMLButtonElement): void {
  const dialog = document.querySelector<HTMLDialogElement>(
    "[data-comment-dialog]",
  );
  const viewportPadding = 8;
  const selectionGap = 8;
  let pointerSelecting = false;
  let updateScheduled = false;

  function hideButton(): void {
    button.hidden = true;
    button.removeAttribute("data-comment-anchor");
  }

  function positionButton(): void {
    updateScheduled = false;

    const selection = currentPageTextSelection();
    if (pointerSelecting || dialog?.open || !selection) {
      hideButton();
      return;
    }

    button.dataset.commentAnchor = selection.text;
    button.hidden = false;
    button.style.left = "0px";
    button.style.top = "0px";

    const buttonRect = button.getBoundingClientRect();
    let left = selection.rect.right + selectionGap;
    let top = selection.rect.bottom + selectionGap;

    if (left + buttonRect.width > window.innerWidth - viewportPadding)
      left = selection.rect.left - buttonRect.width - selectionGap;
    if (top + buttonRect.height > window.innerHeight - viewportPadding)
      top = selection.rect.top - buttonRect.height - selectionGap;

    left = Math.max(
      viewportPadding,
      Math.min(left, window.innerWidth - buttonRect.width - viewportPadding),
    );
    top = Math.max(
      viewportPadding,
      Math.min(top, window.innerHeight - buttonRect.height - viewportPadding),
    );

    button.style.left = `${Math.round(left)}px`;
    button.style.top = `${Math.round(top)}px`;
  }

  function schedulePositionUpdate(): void {
    if (updateScheduled) return;

    updateScheduled = true;
    requestAnimationFrame(positionButton);
  }

  document.addEventListener("selectionchange", schedulePositionUpdate);
  document.addEventListener("pointerdown", (event: PointerEvent) => {
    const target = event.target;
    if (
      target instanceof Element &&
      target.closest("[data-floating-comment-button]")
    )
      return;

    pointerSelecting = true;
    hideButton();
  });
  document.addEventListener("pointerup", () => {
    if (!pointerSelecting) return;

    pointerSelecting = false;
    schedulePositionUpdate();
  });
  window.addEventListener("scroll", hideButton, { passive: true });
  window.addEventListener("resize", hideButton);

  button.addEventListener("pointerdown", (event: PointerEvent) => {
    event.preventDefault();
  });
  button.addEventListener("click", hideButton);
}

type TextPoint = {
  node: Text;
  offset: number;
};

type NormalizedTextIndex = {
  text: string;
  starts: TextPoint[];
  ends: TextPoint[];
};

type InlineCommentGroup = {
  anchor: string;
  threads: HTMLElement[];
  marker: HTMLButtonElement;
};

const inlineCommentBlockSelector =
  "p,li,blockquote,h1,h2,h3,h4,h5,h6,pre,td,th,figcaption,dd,dt";

function normalizedCommentAnchor(value: string): string {
  return value.replace(/\s+/g, " ").trim();
}

function textBlock(node: Text, root: HTMLElement): Element | null {
  const parent = node.parentElement;
  if (!parent) return null;

  return parent.closest(inlineCommentBlockSelector) ?? root;
}

function selectableTextNode(node: Node, root: HTMLElement): node is Text {
  if (!(node instanceof Text) || !node.data) return false;

  const parent = node.parentElement;
  if (!parent || !root.contains(parent)) return false;

  return !parent.closest(
    "button,input,textarea,select,script,style,[contenteditable='true']",
  );
}

function buildNormalizedTextIndex(root: HTMLElement): NormalizedTextIndex {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const starts: TextPoint[] = [];
  const ends: TextPoint[] = [];
  let text = "";
  let previousBlock: Element | null = null;

  function appendSpace(point: TextPoint): void {
    if (!text || text.endsWith(" ")) return;

    text += " ";
    starts.push(point);
    ends.push(point);
  }

  for (let current = walker.nextNode(); current; current = walker.nextNode()) {
    if (!selectableTextNode(current, root)) continue;

    const node = current;
    const block = textBlock(node, root);
    if (previousBlock && block !== previousBlock)
      appendSpace({ node, offset: 0 });

    for (let offset = 0; offset < node.data.length; offset += 1) {
      const character = node.data[offset] ?? "";
      if (/\s/.test(character)) {
        appendSpace({ node, offset });
        continue;
      }

      text += character;
      starts.push({ node, offset });
      ends.push({ node, offset: offset + 1 });
    }

    previousBlock = block;
  }

  return { text: text.trimEnd(), starts, ends };
}

function rangeForCommentAnchor(
  index: NormalizedTextIndex,
  anchor: string,
): Range | null {
  const expected = normalizedCommentAnchor(anchor);
  if (!expected) return null;

  const startIndex = index.text.indexOf(expected);
  if (startIndex < 0) return null;

  const endIndex = startIndex + expected.length - 1;
  const start = index.starts[startIndex];
  const end = index.ends[endIndex];
  if (!start || !end) return null;

  const range = document.createRange();
  range.setStart(start.node, start.offset);
  range.setEnd(end.node, end.offset);
  return range;
}

function createInlineCommentMarker(threads: HTMLElement[]): HTMLButtonElement {
  const marker = document.createElement("button");
  const count = threads.reduce(
    (total, thread) =>
      total + thread.querySelectorAll("[data-inline-comment-item]").length,
    0,
  );
  const allResolved = threads.every(
    (thread) => thread.dataset.inlineCommentResolved === "true",
  );
  const icon = document.querySelector<SVGElement>(
    "[data-floating-comment-button] svg",
  );

  marker.type = "button";
  marker.className = "inline-comment-marker";
  marker.setAttribute("aria-label", `Open inline discussion (${count})`);
  marker.setAttribute("aria-expanded", "false");
  marker.setAttribute("aria-controls", "inline-comment-panel");
  marker.title = `${count} inline ${count === 1 ? "comment" : "comments"}`;
  marker.hidden = true;
  marker.classList.toggle("resolved", allResolved);

  if (icon) marker.append(icon.cloneNode(true));

  const label = document.createElement("span");
  label.textContent = String(count);
  marker.append(label);

  document.body.append(marker);
  return marker;
}

function setupInlineCommentThreads(): void {
  const prose = document.querySelector<HTMLElement>(".page-reading .prose");
  const store = document.querySelector<HTMLElement>(
    "[data-inline-comment-thread-store]",
  );
  const panel = document.querySelector<HTMLElement>(
    "[data-inline-comment-panel]",
  );
  const panelBody = document.querySelector<HTMLElement>(
    "[data-inline-comment-panel-body]",
  );
  const backdrop = document.querySelector<HTMLButtonElement>(
    "[data-inline-comment-panel-backdrop]",
  );
  if (!prose || !store || !panel || !panelBody || !backdrop) return;

  const pageProse = prose;
  const threadStore = store;
  const threadPanel = panel;
  const threadPanelBody = panelBody;
  const panelBackdrop = backdrop;

  const threadElements = Array.from(
    threadStore.querySelectorAll<HTMLElement>("[data-inline-comment-thread]"),
  );
  if (!threadElements.length) return;

  const threadsByAnchor = new Map<string, HTMLElement[]>();
  for (const thread of threadElements) {
    const anchor = normalizedCommentAnchor(
      requiredAttribute(thread, "data-inline-comment-anchor"),
    );
    if (!anchor) continue;

    const threads = threadsByAnchor.get(anchor) ?? [];
    threads.push(thread);
    threadsByAnchor.set(anchor, threads);
  }

  const groups: InlineCommentGroup[] = Array.from(
    threadsByAnchor.entries(),
  ).map(([anchor, threads]) => ({
    anchor,
    threads,
    marker: createInlineCommentMarker(threads),
  }));
  if (!groups.length) return;

  const highlights: HTMLElement[] = [];
  let activeGroup: InlineCommentGroup | null = null;
  let refreshScheduled = false;

  function clearHighlights(): void {
    for (const highlight of highlights) highlight.remove();
    highlights.length = 0;
  }

  function hideActiveThreads(): void {
    if (!activeGroup) return;

    for (const thread of activeGroup.threads) {
      thread.hidden = true;
      threadStore.append(thread);
    }

    activeGroup.marker.setAttribute("aria-expanded", "false");
    activeGroup = null;
  }

  function closePanel(): void {
    const marker = activeGroup?.marker ?? null;

    hideActiveThreads();
    threadPanel.hidden = true;
    panelBackdrop.hidden = true;
    document.body.classList.remove("inline-comment-panel-open");
    marker?.focus();
  }

  function openPanel(group: InlineCommentGroup): void {
    if (activeGroup !== group) {
      hideActiveThreads();
      activeGroup = group;

      for (const thread of group.threads) {
        thread.hidden = false;
        threadPanelBody.append(thread);
      }
    }

    group.marker.setAttribute("aria-expanded", "true");
    threadPanel.hidden = false;
    panelBackdrop.hidden = false;
    document.body.classList.add("inline-comment-panel-open");

    const hash = window.location.hash;
    if (hash.startsWith("#comment-")) {
      const target = threadPanel.querySelector<HTMLElement>(hash);
      requestAnimationFrame(() => target?.scrollIntoView({ block: "nearest" }));
    } else {
      threadPanel.scrollTop = 0;
    }
  }

  function addHighlight(rect: DOMRect): void {
    const highlight = document.createElement("span");
    highlight.className = "inline-comment-anchor-highlight";
    highlight.style.left = `${Math.round(window.scrollX + rect.left)}px`;
    highlight.style.top = `${Math.round(window.scrollY + rect.top)}px`;
    highlight.style.width = `${Math.max(1, Math.round(rect.width))}px`;
    highlight.style.height = `${Math.max(1, Math.round(rect.height))}px`;
    document.body.append(highlight);
    highlights.push(highlight);
  }

  function positionMarker(
    group: InlineCommentGroup,
    rect: DOMRect | null,
    detachedIndex: number,
  ): void {
    const proseRect = pageProse.getBoundingClientRect();
    const marker = group.marker;
    marker.hidden = false;
    marker.classList.toggle("detached", rect === null);
    marker.style.left = "0px";
    marker.style.top = "0px";

    const markerRect = marker.getBoundingClientRect();
    const viewportPadding = 8;
    const gutter = 8;
    const maxLeft =
      window.scrollX + window.innerWidth - markerRect.width - viewportPadding;
    let left = window.scrollX + proseRect.right + gutter;
    let top: number;

    if (rect) {
      top = window.scrollY + rect.top + (rect.height - markerRect.height) / 2;
    } else {
      left = Math.min(left, maxLeft);
      top = window.scrollY + proseRect.bottom + 12 + detachedIndex * 34;
    }

    if (left > maxLeft) {
      const referenceRight = rect?.right ?? proseRect.right;
      left = Math.min(window.scrollX + referenceRight + gutter, maxLeft);
    }

    marker.style.left = `${Math.round(Math.max(viewportPadding, left))}px`;
    marker.style.top = `${Math.round(Math.max(viewportPadding, top))}px`;
  }

  function refreshPositions(): void {
    refreshScheduled = false;
    clearHighlights();

    const textIndex = buildNormalizedTextIndex(pageProse);
    let detachedIndex = 0;
    for (const group of groups) {
      const range = rangeForCommentAnchor(textIndex, group.anchor);
      const rectangles = range
        ? Array.from(range.getClientRects()).filter(
            (rect) => rect.width > 0 && rect.height > 0,
          )
        : [];

      for (const rect of rectangles) addHighlight(rect);

      const lastRect = rectangles.at(-1) ?? null;
      positionMarker(group, lastRect, detachedIndex);
      if (!lastRect) detachedIndex += 1;
    }
  }

  function scheduleRefresh(): void {
    if (refreshScheduled) return;

    refreshScheduled = true;
    requestAnimationFrame(refreshPositions);
  }

  for (const group of groups)
    group.marker.addEventListener("click", () => openPanel(group));

  for (const close of document.querySelectorAll<HTMLButtonElement>(
    "[data-inline-comment-panel-close]",
  ))
    close.addEventListener("click", closePanel);

  threadPanel.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const link = target.closest<HTMLAnchorElement>('a[href^="#comment-"]');
    if (!link) return;

    const comment = threadPanel.querySelector<HTMLElement>(link.hash);
    if (!comment) return;

    event.preventDefault();
    history.replaceState(null, "", link.hash);
    comment.scrollIntoView({ block: "nearest" });
  });

  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key === "Escape" && !threadPanel.hidden) closePanel();
  });
  window.addEventListener("resize", scheduleRefresh);

  if ("ResizeObserver" in window)
    new ResizeObserver(scheduleRefresh).observe(pageProse);

  void document.fonts?.ready.then(scheduleRefresh);
  refreshPositions();

  const requestedComment = window.location.hash.match(/^#comment-(\d+)$/)?.[1];
  if (requestedComment) {
    const group = groups.find((candidate) =>
      candidate.threads.some((thread) =>
        Boolean(
          thread.querySelector(
            `[data-inline-comment-id="${CSS.escape(requestedComment)}"]`,
          ),
        ),
      ),
    );
    if (group) {
      group.marker.scrollIntoView({ block: "center" });
      openPanel(group);
    }
  }
}

function setupCommentDialog(dialog: HTMLDialogElement): void {
  const anchor = requiredElement<HTMLInputElement>(
    dialog,
    "[data-comment-anchor]",
  );
  const kind = requiredElement<HTMLInputElement>(dialog, "[data-comment-kind]");
  const body = requiredElement<HTMLTextAreaElement>(
    dialog,
    'textarea[name="body"]',
  );
  const replacement = requiredElement<HTMLTextAreaElement>(
    dialog,
    "[data-comment-replacement]",
  );
  const parentID = requiredElement<HTMLInputElement>(
    dialog,
    "[data-comment-parent-id]",
  );
  const quote = requiredElement<HTMLInputElement>(
    dialog,
    "[data-comment-quote]",
  );
  const dialogTitle = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-dialog-title]",
  );
  const replyContext = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-reply-context]",
  );
  const replyAuthor = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-reply-author]",
  );
  const replyExcerpt = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-reply-excerpt]",
  );
  const quotePreview = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-quote-preview]",
  );
  const selectedContext = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-selected-context]",
  );
  const selectedText = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-selected-text]",
  );
  const modeSwitch = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-mode-switch]",
  );
  const modeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-comment-compose-mode]",
  );
  const suggestionField = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-suggestion-field]",
  );
  const bodyLabel = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-body-label]",
  );
  const clearReply = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-comment-reply-clear]",
  );
  const submitLabel = requiredElement<HTMLElement>(
    dialog,
    "[data-comment-submit-label]",
  );
  const openButtons = requiredElements<HTMLButtonElement>(
    document,
    "[data-comment-dialog-open]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-comment-dialog-close]",
  );

  function setSelectedAnchor(value: string): void {
    const selected = value.trim();

    anchor.value = selected;
    selectedText.textContent = selected;
    selectedContext.hidden = !selected;
    modeSwitch.hidden = !selected || Boolean(parentID.value);

    if (replacement.dataset.commentSourceAnchor !== selected) {
      replacement.value = selected;
      replacement.dataset.commentSourceAnchor = selected;
    }
  }

  function setComposeMode(mode: "comment" | "suggestion"): void {
    const suggestion = mode === "suggestion" && Boolean(anchor.value);

    kind.value = suggestion ? "suggestion" : "comment";
    suggestionField.hidden = !suggestion;
    body.required = !suggestion;
    bodyLabel.textContent = suggestion ? "Comment (optional)" : "Comment";
    body.placeholder = suggestion
      ? "Explain why this change would help, or mention @username…"
      : "Add context or mention @username…";
    dialogTitle.textContent = suggestion
      ? "Suggest change"
      : parentID.value
        ? "Reply"
        : anchor.value
          ? "Add inline comment"
          : "Add comment";
    submitLabel.textContent = suggestion
      ? "Create suggestion"
      : parentID.value
        ? "Reply"
        : "Add comment";

    for (const button of modeButtons) {
      const active = button.dataset.commentComposeMode === kind.value;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    }
  }

  function clearReplyContext(): void {
    parentID.value = "";
    quote.value = "";
    replyAuthor.textContent = "";
    replyExcerpt.textContent = "";
    replyContext.hidden = true;
    quotePreview.textContent = "";
    quotePreview.hidden = true;
    modeSwitch.hidden = !anchor.value;
    setComposeMode("comment");
  }

  function prepareReply(button: HTMLButtonElement): void {
    const id = button.dataset.commentParentId?.trim() ?? "";
    if (!id) {
      clearReplyContext();
      return;
    }

    const author = button.dataset.commentAuthor?.trim() || "comment author";
    const source = button.dataset.commentBody?.trim() ?? "";
    const excerpt = source.slice(0, 220);

    parentID.value = id;
    replyAuthor.textContent = author;
    replyExcerpt.textContent = excerpt;
    replyContext.hidden = false;
    setSelectedAnchor("");
    modeSwitch.hidden = true;

    if (button.dataset.commentMode === "quote") {
      quote.value = source.slice(0, 500);
      quotePreview.textContent = quote.value;
      quotePreview.hidden = false;
    } else {
      quote.value = "";
      quotePreview.textContent = "";
      quotePreview.hidden = true;
    }

    setComposeMode("comment");
  }

  for (const button of modeButtons) {
    button.addEventListener("click", () => {
      const mode = button.dataset.commentComposeMode;
      if (mode === "comment" || mode === "suggestion") setComposeMode(mode);
    });
  }

  for (const button of openButtons) {
    button.addEventListener("click", () => {
      prepareReply(button);
      if (!parentID.value) {
        const selected =
          button.dataset.commentAnchor?.trim() || selectedPageText();
        setSelectedAnchor(selected);
        setComposeMode("comment");
      }

      dialog.showModal();
      requestAnimationFrame(() => body.focus());
    });
  }

  clearReply.addEventListener("click", () => clearReplyContext());

  for (const button of closeButtons)
    button.addEventListener("click", () => dialog.close());

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}
