// Read-only layout behavior for filesystem-backed Kumbuka sites.

const MOBILE_SIDEBAR_QUERY = "(max-width: 800px)";
const SIDEBAR_HIDDEN_KEY = "kumbuka:static-sidebar-hidden";

function initNavigationTree(): void {
  const navigation = document.querySelector<HTMLElement>("[data-navigation]");
  if (!navigation) return;

  const nodes = [
    ...navigation.querySelectorAll<HTMLDetailsElement>(
      "details[data-nav-node]",
    ),
  ];

  navigation
    .querySelector<HTMLButtonElement>("[data-navigation-collapse-all]")
    ?.addEventListener("click", () => {
      for (const node of nodes) node.open = false;
    });
  navigation
    .querySelector<HTMLButtonElement>("[data-navigation-expand-all]")
    ?.addEventListener("click", () => {
      for (const node of nodes) node.open = true;
    });

  const active = navigation.querySelector<HTMLElement>('[aria-current="page"]');
  if (!active) return;

  const navigationRect = navigation.getBoundingClientRect();
  const activeRect = active.getBoundingClientRect();

  if (
    activeRect.top < navigationRect.top ||
    activeRect.bottom > navigationRect.bottom
  ) {
    navigation.scrollTop +=
      activeRect.top - navigationRect.top - navigation.clientHeight / 3;
  }
}

function initMobileSidebar(): void {
  const trigger = document.querySelector<HTMLButtonElement>("[data-menu]");
  const sidebar = document.querySelector<HTMLElement>("[data-sidebar]");
  const backdrop = document.querySelector<HTMLButtonElement>(
    "[data-sidebar-backdrop]",
  );
  if (!trigger || !sidebar || !backdrop) return;

  const menuTrigger = trigger;
  const sidebarElement = sidebar;
  const sidebarBackdrop = backdrop;
  const mobile = window.matchMedia(MOBILE_SIDEBAR_QUERY);

  function setOpen(open: boolean, restoreFocus = false): void {
    const nextOpen = open && mobile.matches;

    sidebarElement.classList.toggle("open", nextOpen);
    document.body.classList.toggle("sidebar-open", nextOpen);
    menuTrigger.setAttribute("aria-expanded", String(nextOpen));
    sidebarBackdrop.hidden = !nextOpen;

    if (restoreFocus) menuTrigger.focus();
  }

  menuTrigger.addEventListener("click", () => {
    setOpen(!sidebarElement.classList.contains("open"));
  });
  sidebarBackdrop.addEventListener("click", () => setOpen(false, true));
  sidebarElement.addEventListener("click", (event: MouseEvent) => {
    if (!mobile.matches) return;

    const target = event.target;

    if (target instanceof Element && target.closest("a[href]")) setOpen(false);
  });
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "Escape" || !sidebarElement.classList.contains("open"))
      return;

    event.preventDefault();
    setOpen(false, true);
  });
  mobile.addEventListener("change", () => setOpen(false));
}

function initSidebarVisibility(): void {
  const sidebar = document.querySelector<HTMLElement>("[data-sidebar]");
  const toggle = document.querySelector<HTMLButtonElement>(
    "[data-sidebar-visibility-toggle]",
  );
  if (!sidebar || !toggle) return;

  const visibilityToggle = toggle;
  const mobile = window.matchMedia(MOBILE_SIDEBAR_QUERY);

  function sync(): void {
    if (mobile.matches) {
      document.documentElement.classList.remove("sidebar-hidden");
    } else {
      try {
        document.documentElement.classList.toggle(
          "sidebar-hidden",
          window.localStorage.getItem(SIDEBAR_HIDDEN_KEY) === "true",
        );
      } catch {
        // Storage may be unavailable in restricted browser contexts.
      }
    }

    const hidden =
      !mobile.matches &&
      document.documentElement.classList.contains("sidebar-hidden");

    visibilityToggle.setAttribute("aria-expanded", String(!hidden));

    const label = hidden ? "Show navigation" : "Hide navigation";

    visibilityToggle.setAttribute("aria-label", label);
    visibilityToggle.title = label;
  }

  visibilityToggle.addEventListener("click", () => {
    if (mobile.matches) return;

    const hidden = document.documentElement.classList.toggle("sidebar-hidden");

    try {
      if (hidden) window.localStorage.setItem(SIDEBAR_HIDDEN_KEY, "true");
      else window.localStorage.removeItem(SIDEBAR_HIDDEN_KEY);
    } catch {
      // Storage may be unavailable in restricted browser contexts.
    }

    sync();
  });
  mobile.addEventListener("change", sync);
  sync();
}

export function initStaticLayout(): void {
  initMobileSidebar();
  initSidebarVisibility();
  initNavigationTree();
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (
      event.key === "/" &&
      !["INPUT", "TEXTAREA", "SELECT"].includes(
        document.activeElement?.tagName ?? "",
      )
    ) {
      event.preventDefault();
      document.querySelector<HTMLInputElement>(".global-search input")?.focus();
    }
  });
}
