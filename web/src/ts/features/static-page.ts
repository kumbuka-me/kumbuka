// Read-only page contents behavior for filesystem-backed Kumbuka sites.

export function initStaticPage(): void {
  const pageHeading = document.querySelector<HTMLElement>(".page-heading");
  const pageReading = document.querySelector<HTMLElement>(".page-reading");
  const pageContents = document.querySelector<HTMLElement>(
    "[data-page-contents]",
  );
  const toggle = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-toggle]",
  );
  const backdrop = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-backdrop]",
  );
  const close = document.querySelector<HTMLButtonElement>(
    "[data-page-contents-close]",
  );
  const mobile = window.matchMedia("(max-width: 800px)");
  let desktopVisible =
    pageReading?.classList.contains("with-contents") ?? false;

  function updateHeadingHeight(): void {
    const sticky = window.matchMedia("(min-width: 801px)").matches;
    const height = sticky && pageHeading ? pageHeading.offsetHeight : 0;

    document.documentElement.style.setProperty(
      "--page-heading-height",
      `${height}px`,
    );
  }

  updateHeadingHeight();
  window.addEventListener("resize", updateHeadingHeight);

  if (pageHeading && "ResizeObserver" in window) {
    new ResizeObserver(updateHeadingHeight).observe(pageHeading);
  }

  let scheduleActiveHeadingUpdate = (): void => {};

  if (pageContents) {
    const entries = [
      ...pageContents.querySelectorAll<HTMLAnchorElement>('a[href^="#"]'),
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

    function setActive(link: HTMLAnchorElement): void {
      if (activeLink === link) return;

      activeLink?.classList.remove("active");
      activeLink?.removeAttribute("aria-current");
      link.classList.add("active");
      link.setAttribute("aria-current", "location");
      activeLink = link;
    }

    function updateActive(): void {
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

      setActive(current.link);
    }

    scheduleActiveHeadingUpdate = () => {
      if (scheduled) return;

      scheduled = true;
      requestAnimationFrame(updateActive);
    };
    window.addEventListener("scroll", scheduleActiveHeadingUpdate, {
      passive: true,
    });
    window.addEventListener("resize", scheduleActiveHeadingUpdate);
    updateActive();
  }

  function setDesktopVisible(show: boolean): void {
    if (!pageReading || !toggle) return;

    desktopVisible = show;
    pageReading.classList.toggle("with-contents", show);
    pageReading.classList.toggle("contents-hidden", !show);
    toggle.setAttribute("aria-pressed", String(show));

    const label = show ? "Hide page contents" : "Show page contents";

    toggle.setAttribute("aria-label", label);
    toggle.title = label;

    if (show) scheduleActiveHeadingUpdate();
  }

  function setMobileOpen(open: boolean, restoreFocus = false): void {
    if (!pageReading || !pageContents || !toggle) return;

    const nextOpen = open && mobile.matches;

    pageReading.classList.toggle("contents-popover-open", nextOpen);
    document.body.classList.toggle("contents-popover-open", nextOpen);

    if (backdrop) backdrop.hidden = !nextOpen;

    toggle.setAttribute("aria-expanded", String(nextOpen));
    pageContents.setAttribute("aria-hidden", String(!nextOpen));

    if (nextOpen) close?.focus();
    else if (restoreFocus) toggle.focus();
  }

  function syncMode(): void {
    setMobileOpen(false);
    if (!pageContents || !toggle) return;
    if (mobile.matches) {
      toggle.removeAttribute("aria-pressed");
      pageContents.setAttribute("aria-hidden", "true");
      return;
    }

    pageContents.removeAttribute("aria-hidden");
    setDesktopVisible(desktopVisible);
  }

  toggle?.addEventListener("click", () => {
    if (!pageReading) return;
    if (mobile.matches) {
      setMobileOpen(!pageReading.classList.contains("contents-popover-open"));
      return;
    }

    setDesktopVisible(!pageReading.classList.contains("with-contents"));
  });
  close?.addEventListener("click", () => setMobileOpen(false, true));
  backdrop?.addEventListener("click", () => setMobileOpen(false, true));
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (
      event.key === "Escape" &&
      pageReading?.classList.contains("contents-popover-open")
    ) {
      event.preventDefault();
      setMobileOpen(false, true);
    }
  });
  mobile.addEventListener("change", syncMode);
  syncMode();
}
