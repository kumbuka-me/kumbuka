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
  if (commentDialog) setupCommentDialog(commentDialog);
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

function selectedPageText(): string {
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || !selection.rangeCount) return "";

  const range = selection.getRangeAt(0);
  const prose = document.querySelector<HTMLElement>(".page-reading .prose");
  if (!prose || !prose.contains(range.commonAncestorContainer)) return "";

  return selection.toString().trim().slice(0, 500);
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
  const openButtons = requiredElements<HTMLButtonElement>(
    document,
    "[data-comment-dialog-open]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-comment-dialog-close]",
  );

  for (const button of openButtons) {
    button.addEventListener("click", () => {
      if (!anchor.value.trim()) anchor.value = selectedPageText();

      dialog.showModal();
      requestAnimationFrame(() => body.focus());
    });
  }

  for (const button of closeButtons)
    button.addEventListener("click", () => dialog.close());

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}
