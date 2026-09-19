// Rendered page interactions and page-local state.

import {
  requiredAttribute,
  requiredElement,
  requiredElements,
} from "../core/dom.ts";

export function initPage(): void {
  const pageHeading = document.querySelector<HTMLElement>(".page-heading");
  const pageReading = document.querySelector<HTMLElement>(".page-reading");
  const pageContents = document.querySelector<HTMLElement>(
    "[data-page-contents]",
  );
  const pageContentsToggle = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-toggle]",
  );
  const pageContentsBackdrop = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-backdrop]",
  );
  const pageContentsClose = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-close]",
  );
  const mobileContents = window.matchMedia("(max-width: 800px)");
  let desktopContentsVisible =
    pageReading?.classList.contains("with-contents") ?? false;
  let scheduleActiveHeadingUpdate: () => void = () => {};

  function updatePageHeadingHeight(): void {
    const sticky = window.matchMedia("(min-width: 801px)").matches;
    const height = sticky && pageHeading ? pageHeading.offsetHeight : 0;

    document.documentElement.style.setProperty(
      "--page-heading-height",
      `${height}px`,
    );
  }

  updatePageHeadingHeight();
  window.addEventListener("resize", updatePageHeadingHeight);

  if (pageHeading && "ResizeObserver" in window)
    new ResizeObserver(updatePageHeadingHeight).observe(pageHeading);

  if (pageContents) {
    const contents = pageContents;
    const entries = [
      ...contents.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'),
    ]
      .map((link) => {
        const heading = document.getElementById(
          decodeURIComponent(link.hash.slice(1)),
        );
        return heading ? { heading, link } : null;
      })
      .filter(
        (entry): entry is { heading: HTMLElement; link: HTMLAnchorElement } =>
          entry !== null,
      );

    let activeLink: HTMLAnchorElement | null = null;
    let scheduled = false;

    function revealActiveLink(link: HTMLAnchorElement): void {
      if (contents.offsetParent === null) return;

      const container = contents.getBoundingClientRect();
      const item = link.getBoundingClientRect();

      if (item.top < container.top)
        contents.scrollTop -= container.top - item.top;
      else if (item.bottom > container.bottom)
        contents.scrollTop += item.bottom - container.bottom;
    }

    function setActiveHeading(link: HTMLAnchorElement): void {
      if (activeLink === link) return;

      activeLink?.classList.remove("active");
      activeLink?.removeAttribute("aria-current");
      link.classList.add("active");
      link.setAttribute("aria-current", "location");
      activeLink = link;
      revealActiveLink(link);
    }

    function updateActiveHeading(): void {
      scheduled = false;

      const first = entries[0];
      if (!first) return;

      const headingBottom = pageHeading?.getBoundingClientRect().bottom ?? 0;
      const threshold = Math.max(96, headingBottom + 12);
      let current = first;

      for (const entry of entries) {
        if (entry.heading.getBoundingClientRect().top <= threshold)
          current = entry;
        else break;
      }
      if (
        window.innerHeight + window.scrollY >=
        document.documentElement.scrollHeight - 2
      ) {
        current = entries.at(-1) ?? current;
      }

      setActiveHeading(current.link);
    }

    scheduleActiveHeadingUpdate = () => {
      if (scheduled) return;

      scheduled = true;
      requestAnimationFrame(updateActiveHeading);
    };

    window.addEventListener("scroll", scheduleActiveHeadingUpdate, {
      passive: true,
    });
    window.addEventListener("resize", scheduleActiveHeadingUpdate);
    updateActiveHeading();
  }

  async function persistPageContentsPreference(show: boolean): Promise<void> {
    if (!pageContentsToggle) return;

    const url = requiredAttribute(pageContentsToggle, "data-preference-url");
    const response = await fetch(url, {
      method: "POST",
      headers: {
        Accept: "text/plain",
        "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
      },
      body: new URLSearchParams({ show: String(show) }),
    });
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
  }

  function setPageContentsVisible(show: boolean): void {
    if (!pageReading || !pageContentsToggle) return;

    desktopContentsVisible = show;
    pageReading.classList.toggle("with-contents", show);
    pageReading.classList.toggle("contents-hidden", !show);
    pageContentsToggle.setAttribute("aria-pressed", String(show));

    const label = show ? "Hide page contents" : "Show page contents";

    pageContentsToggle.setAttribute("aria-label", label);
    pageContentsToggle.title = label;

    if (show) scheduleActiveHeadingUpdate();
  }

  function setContentsPopoverOpen(open: boolean, restoreFocus = false): void {
    if (!pageReading || !pageContents || !pageContentsToggle) return;

    const nextOpen = open && mobileContents.matches;

    pageReading.classList.toggle("contents-popover-open", nextOpen);
    document.body.classList.toggle("contents-popover-open", nextOpen);

    if (pageContentsBackdrop) pageContentsBackdrop.hidden = !nextOpen;

    pageContentsToggle.setAttribute("aria-expanded", String(nextOpen));
    pageContents.setAttribute("aria-hidden", String(!nextOpen));

    const label = nextOpen ? "Hide page contents" : "Show page contents";

    pageContentsToggle.setAttribute("aria-label", label);
    pageContentsToggle.title = label;

    if (nextOpen) pageContentsClose?.focus();
    else if (restoreFocus) pageContentsToggle.focus();
  }

  function syncContentsMode(): void {
    setContentsPopoverOpen(false);
    if (!pageContents || !pageContentsToggle) return;
    if (mobileContents.matches) {
      pageContentsToggle.removeAttribute("aria-pressed");
      pageContents.setAttribute("aria-hidden", "true");
      return;
    }

    pageContents.removeAttribute("aria-hidden");
    pageContentsToggle.setAttribute(
      "aria-pressed",
      String(desktopContentsVisible),
    );

    const label = desktopContentsVisible
      ? "Hide page contents"
      : "Show page contents";

    pageContentsToggle.setAttribute("aria-label", label);
    pageContentsToggle.title = label;
  }

  pageContentsToggle?.addEventListener("click", async () => {
    if (!pageReading || !pageContentsToggle) return;
    if (mobileContents.matches) {
      setContentsPopoverOpen(
        !pageReading.classList.contains("contents-popover-open"),
      );
      return;
    }

    const previous = pageReading.classList.contains("with-contents");
    const next = !previous;

    setPageContentsVisible(next);
    pageContentsToggle.disabled = true;

    try {
      await persistPageContentsPreference(next);
    } catch (error) {
      console.error("failed to save page contents preference", error);
      setPageContentsVisible(previous);
    } finally {
      pageContentsToggle.disabled = false;
    }
  });
  pageContentsBackdrop?.addEventListener("click", () =>
    setContentsPopoverOpen(false, true),
  );
  pageContentsClose?.addEventListener("click", () =>
    setContentsPopoverOpen(false, true),
  );
  pageContents?.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (
      mobileContents.matches &&
      target instanceof Element &&
      target.closest('a[href^="#"]')
    )
      setContentsPopoverOpen(false);
  });
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (
      event.key !== "Escape" ||
      !pageReading?.classList.contains("contents-popover-open")
    )
      return;

    event.preventDefault();
    setContentsPopoverOpen(false, true);
  });
  mobileContents.addEventListener("change", syncContentsMode);
  syncContentsMode();

  const widgetDialog = document.querySelector<HTMLDialogElement>(
    "[data-widget-dialog]",
  );
  if (widgetDialog) setupWidgetDialog(widgetDialog);

  const moveDialog = document.querySelector<HTMLDialogElement>(
    "[data-move-page-dialog]",
  );
  if (moveDialog) setupMoveDialog(moveDialog);

  const commentDialog = document.querySelector<HTMLDialogElement>(
    "[data-comment-dialog]",
  );
  if (commentDialog) {
    setupCommentDialog(commentDialog);

    const floatingCommentButton = document.querySelector<HTMLButtonElement>(
      "[data-floating-comment-button]",
    );
    if (floatingCommentButton)
      setupFloatingCommentButton(floatingCommentButton);
  }
}

function setupWidgetDialog(dialog: HTMLDialogElement): void {
  const opens = Array.from(
    document.querySelectorAll<HTMLButtonElement>("[data-widget-dialog-open]"),
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-widget-dialog-close]",
  );
  const title = requiredElement<HTMLElement>(
    dialog,
    "[data-widget-dialog-title]",
  );
  const body = requiredElement<HTMLElement>(
    dialog,
    "[data-widget-dialog-body]",
  );
  let loadedURL = "";

  async function load(button: HTMLButtonElement): Promise<void> {
    const url = requiredAttribute(button, "data-widget-dialog-url");
    if (loadedURL === url) return;

    button.disabled = true;
    body.innerHTML = '<p class="muted">Loading…</p>';

    try {
      const response = await fetch(url, { headers: { Accept: "text/html" } });
      if (!response.ok) throw new Error(`HTTP ${response.status}`);

      body.innerHTML = await response.text();
      loadedURL = url;
    } catch (error) {
      console.error("failed to load widget dialog", error);
      body.innerHTML =
        '<p class="widget-dialog-error">The content could not be loaded. Close the dialog and try again.</p>';
    } finally {
      button.disabled = false;
    }
  }

  for (const button of opens) {
    button.addEventListener("click", () => {
      title.textContent = requiredAttribute(button, "data-widget-dialog-title");
      dialog.showModal();
      void load(button);
    });
  }

  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}

function setupMoveDialog(dialog: HTMLDialogElement): void {
  const openButtons = requiredElements<HTMLButtonElement>(
    document,
    "[data-move-page-open]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-move-page-close]",
  );

  for (const button of openButtons)
    button.addEventListener("click", () => dialog.showModal());

  for (const button of closeButtons)
    button.addEventListener("click", () => dialog.close());

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
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

function setupCommentDialog(dialog: HTMLDialogElement): void {
  const anchor = requiredElement<HTMLTextAreaElement>(
    dialog,
    "[data-comment-anchor]",
  );
  const body = requiredElement<HTMLTextAreaElement>(
    dialog,
    'textarea[name="body"]',
  );
  const parentID = requiredElement<HTMLInputElement>(
    dialog,
    "[data-comment-parent-id]",
  );
  const quote = requiredElement<HTMLInputElement>(
    dialog,
    "[data-comment-quote]",
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

  function clearReplyContext(): void {
    parentID.value = "";
    quote.value = "";
    replyAuthor.textContent = "";
    replyExcerpt.textContent = "";
    replyContext.hidden = true;
    quotePreview.textContent = "";
    quotePreview.hidden = true;
    submitLabel.textContent = "Add comment";
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
    submitLabel.textContent = "Reply";
    anchor.value = "";

    if (button.dataset.commentMode === "quote") {
      quote.value = source.slice(0, 500);
      quotePreview.textContent = quote.value;
      quotePreview.hidden = false;
    } else {
      quote.value = "";
      quotePreview.textContent = "";
      quotePreview.hidden = true;
    }
  }

  for (const button of openButtons) {
    button.addEventListener("click", () => {
      prepareReply(button);
      if (!parentID.value)
        anchor.value =
          button.dataset.commentAnchor?.trim() || selectedPageText();

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


