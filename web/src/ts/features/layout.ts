// Sidebar, navigation tree, and account-menu behavior.

import { requiredAttribute } from "../core/dom.ts";

const MIN_SIDEBAR_WIDTH = 220;
const MAX_SIDEBAR_WIDTH = 420;
const SIDEBAR_WIDTH_PRESETS = new Map<number, string>([
  [240, "Narrow"],
  [280, "Standard"],
  [360, "Wide"],
]);

const MOBILE_SIDEBAR_QUERY = "(max-width: 800px)";
const NAVIGATION_SCROLL_KEY = "kumbuka:navigation-scroll";
const SIDEBAR_HIDDEN_KEY = "kumbuka:sidebar-hidden";

type AccountMenu = HTMLElement;

function shouldHandleNavigationClick(
  event: MouseEvent,
  link: HTMLAnchorElement | null,
): link is HTMLAnchorElement {
  if (!link || event.defaultPrevented || event.button !== 0) return false;
  const modified =
    event.metaKey || event.ctrlKey || event.shiftKey || event.altKey;
  if (modified) return false;
  if (link.target === "_blank" || link.hasAttribute("download")) return false;

  return true;
}

function clampSidebarWidth(width: number): number {
  return Math.min(MAX_SIDEBAR_WIDTH, Math.max(MIN_SIDEBAR_WIDTH, width));
}

export function sidebarWidthStatus(width: number): string {
  const next = clampSidebarWidth(Math.round(width));
  return `${SIDEBAR_WIDTH_PRESETS.get(next) ?? "Custom"} · ${next} px`;
}

function syncSidebarWidthSetting(width: number): number {
  const next = clampSidebarWidth(Math.round(width));
  const input = document.querySelector<HTMLInputElement>(
    "[data-sidebar-width-setting]",
  );
  const output = document.querySelector<HTMLOutputElement>(
    "[data-sidebar-width-output]",
  );

  if (input) input.value = String(next);
  if (output) {
    const status = sidebarWidthStatus(next);

    output.value = status;
    output.textContent = status;
  }
  for (const preset of document.querySelectorAll<HTMLButtonElement>(
    "[data-sidebar-width-preset]",
  )) {
    const selected = Number(preset.dataset.sidebarWidthPreset) === next;
    preset.setAttribute("aria-pressed", String(selected));
  }

  return next;
}

async function postJSON(url: string, value: unknown): Promise<void> {
  const response = await fetch(url, {
    method: "POST",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(value),
  });
  if (!response.ok) throw new Error(`HTTP ${response.status}`);
}

function initNavigationTree(): void {
  const navigation = document.querySelector<HTMLElement>("[data-navigation]");
  if (!navigation) return;

  const navigationRoot = navigation;
  const nodes = [
    ...navigation.querySelectorAll<HTMLDetailsElement>(
      "details[data-nav-node]",
    ),
  ];
  const remember = navigation.dataset.navigationRemember?.trim() === "true";
  const stateURL = requiredAttribute(
    navigationRoot,
    "data-navigation-state-url",
  );
  const scrollKey = `${NAVIGATION_SCROLL_KEY}:${window.matchMedia(MOBILE_SIDEBAR_QUERY).matches ? "mobile" : "desktop"}`;
  let saveTimer: ReturnType<typeof setTimeout> | undefined;
  let savePromise: Promise<void> = Promise.resolve();
  let unfilteredState: Map<HTMLDetailsElement, boolean> | undefined;
  const sidebar = navigation.closest<HTMLElement>("[data-sidebar]");
  const filter = sidebar?.querySelector<HTMLInputElement>("[data-tree-filter]");
  const status = sidebar?.querySelector<HTMLElement>(
    "[data-tree-filter-status]",
  );
  const entries = [
    ...navigation.querySelectorAll<HTMLElement>(".nav-node, .nav-page-link"),
  ];

  function filterTree(): void {
    const query = filter?.value.trim().toLocaleLowerCase() ?? "";
    if (query && !unfilteredState)
      unfilteredState = new Map(nodes.map((node) => [node, node.open]));
    let matches = 0;
    for (const entry of [...entries].reverse()) {
      const label =
        entry instanceof HTMLDetailsElement
          ? entry.querySelector(
              "summary .nav-node-link, summary .nav-node-label",
            )?.textContent
          : entry.textContent;
      const ownMatch =
        !query || (label ?? "").toLocaleLowerCase().includes(query);
      const childMatch =
        entry instanceof HTMLDetailsElement &&
        [
          ...entry.querySelectorAll<HTMLElement>(".nav-node, .nav-page-link"),
        ].some((child) => !child.hidden);
      entry.hidden = !ownMatch && !childMatch;
      if (query && ownMatch) matches++;
      if (entry instanceof HTMLDetailsElement) {
        if (query) entry.open = Boolean(childMatch);
        else if (unfilteredState)
          entry.open = unfilteredState.get(entry) ?? false;
      }
    }
    if (!query) unfilteredState = undefined;
    if (status) {
      status.hidden = !query;
      status.textContent = matches
        ? `${matches} matching ${matches === 1 ? "entry" : "entries"}`
        : "No matching pages";
    }
  }
  filter?.addEventListener("input", filterTree);
  filter?.addEventListener("keydown", (event) => {
    if (event.key === "Escape") {
      event.preventDefault();
      event.stopPropagation();
      filter.value = "";
      filterTree();
    }
  });
  const reveal =
    sidebar?.querySelector<HTMLButtonElement>("[data-tree-reveal]");
  const currentPage = navigation.querySelector<HTMLElement>(
    '[aria-current="page"]',
  );
  if (reveal) reveal.disabled = !currentPage;
  function revealCurrentPage(): void {
    if (!currentPage) return;
    if (filter) filter.value = "";
    filterTree();
    for (
      let parent = currentPage.parentElement;
      parent && parent !== navigation;
      parent = parent.parentElement
    )
      if (parent instanceof HTMLDetailsElement) parent.open = true;
    currentPage.scrollIntoView({ block: "nearest" });
  }
  reveal?.addEventListener("click", revealCurrentPage);
  document.addEventListener("navigation-style-change", () => {
    if (filter) filter.value = "";
    filterTree();
    if (document.body.dataset.navigationStyle === "tree") revealCurrentPage();
  });

  function saveScrollPosition(destination: string): void {
    try {
      const url = new URL(destination, window.location.href);
      window.sessionStorage.setItem(
        scrollKey,
        JSON.stringify({
          destination: `${url.pathname}${url.search}`,
          scrollTop: navigationRoot.scrollTop,
        }),
      );
    } catch {
      // Storage can be unavailable in privacy-restricted browser contexts.
    }
  }

  function expandedPaths(): string[] {
    return nodes
      .filter((node) => unfilteredState?.get(node) ?? node.open)
      .map((node) => node.dataset.navPath)
      .filter((value): value is string => Boolean(value));
  }

  function saveExpandedState(): Promise<void> {
    if (!remember) return Promise.resolve();

    const expanded = expandedPaths();

    savePromise = savePromise
      .then(() => postJSON(stateURL, { expanded }))
      .catch((error: unknown) => {
        console.error("failed to save navigation state", error);
      });
    return savePromise;
  }

  function persistExpandedState(): void {
    if (!remember || unfilteredState) return;

    if (saveTimer) clearTimeout(saveTimer);

    saveTimer = setTimeout(() => {
      saveTimer = undefined;
      void saveExpandedState();
    }, 180);
  }

  async function flushExpandedState(): Promise<void> {
    if (saveTimer) {
      clearTimeout(saveTimer);
      saveTimer = undefined;
      await saveExpandedState();
      return;
    }
    await savePromise;
  }

  for (const node of nodes)
    node.addEventListener("toggle", persistExpandedState);

  navigation.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const link = target.closest<HTMLAnchorElement>("a[href]");
    if (!shouldHandleNavigationClick(event, link)) return;

    saveScrollPosition(link.href);
    event.preventDefault();
    void flushExpandedState().finally(() => window.location.assign(link.href));
  });
  navigation
    .querySelector<HTMLElement>("[data-navigation-collapse-all]")
    ?.addEventListener("click", () => {
      for (const node of nodes) node.open = false;
      persistExpandedState();
    });
  navigation
    .querySelector<HTMLElement>("[data-navigation-expand-all]")
    ?.addEventListener("click", () => {
      for (const node of nodes) node.open = true;
      persistExpandedState();
    });

  const restoredScrollPosition =
    navigation.dataset.navigationScrollRestored === "true";
  const active = navigation.querySelector<HTMLElement>('[aria-current="page"]');

  if (document.body.dataset.navigationStyle === "tree") revealCurrentPage();

  if (active && !restoredScrollPosition) {
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
}

function initAccountMenus(): void {
  const menus = [
    ...document.querySelectorAll<AccountMenu>("[data-account-menu]"),
  ];
  if (!menus.length) return;

  function menuItems(menu: AccountMenu): HTMLElement[] {
    return [...menu.querySelectorAll<HTMLElement>('[role="menuitem"]')];
  }

  function closeMenu(menu: AccountMenu, restoreFocus = false): void {
    const trigger = menu.querySelector<HTMLButtonElement>(
      "[data-account-menu-trigger]",
    );
    const popover = menu.querySelector<HTMLElement>(
      "[data-account-menu-popover]",
    );
    if (!trigger || !popover || popover.hidden) return;

    popover.hidden = true;
    trigger.setAttribute("aria-expanded", "false");

    if (restoreFocus) trigger.focus();
  }

  function openMenu(
    menu: AccountMenu,
    focus: "" | "first" | "last" = "",
  ): void {
    for (const other of menus) if (other !== menu) closeMenu(other);

    const trigger = menu.querySelector<HTMLButtonElement>(
      "[data-account-menu-trigger]",
    );
    const popover = menu.querySelector<HTMLElement>(
      "[data-account-menu-popover]",
    );
    if (!trigger || !popover) return;

    popover.hidden = false;
    trigger.setAttribute("aria-expanded", "true");

    const items = menuItems(menu);

    if (focus === "first") items[0]?.focus();
    if (focus === "last") items.at(-1)?.focus();
  }

  for (const menu of menus) {
    const trigger = menu.querySelector<HTMLButtonElement>(
      "[data-account-menu-trigger]",
    );
    const popover = menu.querySelector<HTMLElement>(
      "[data-account-menu-popover]",
    );
    if (!trigger || !popover) continue;

    trigger.addEventListener("click", () =>
      popover.hidden ? openMenu(menu) : closeMenu(menu),
    );
    trigger.addEventListener("keydown", (event: KeyboardEvent) => {
      if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;

      event.preventDefault();
      openMenu(menu, event.key === "ArrowDown" ? "first" : "last");
    });
    popover.addEventListener("keydown", (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeMenu(menu, true);
        return;
      }
      if (!["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) return;

      const items = menuItems(menu);
      if (!items.length) return;

      event.preventDefault();

      const current = items.indexOf(document.activeElement as HTMLElement);
      let next = current;

      if (event.key === "Home") next = 0;
      if (event.key === "End") next = items.length - 1;
      if (event.key === "ArrowDown")
        next = (current + 1 + items.length) % items.length;
      if (event.key === "ArrowUp")
        next = (current - 1 + items.length) % items.length;

      items[next]?.focus();
    });
  }

  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Node)) return;

    for (const menu of menus) if (!menu.contains(target)) closeMenu(menu);
  });
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;
    for (const menu of menus) closeMenu(menu, true);
  });
}

function initMobileSidebar(): void {
  const trigger = document.querySelector<HTMLButtonElement>("[data-menu]");
  const sidebar = document.querySelector<HTMLElement>("[data-sidebar]");
  const backdrop = document.querySelector<HTMLButtonElement>(
    "[data-sidebar-backdrop]",
  );
  if (!trigger || !sidebar || !backdrop) return;

  const menuTrigger = trigger;
  const sidebarRoot = sidebar;
  const backdropButton = backdrop;

  const mobile = window.matchMedia(MOBILE_SIDEBAR_QUERY);

  function setOpen(open: boolean, restoreFocus = false): void {
    const nextOpen = open && mobile.matches;

    sidebarRoot.classList.toggle("open", nextOpen);
    document.body.classList.toggle("sidebar-open", nextOpen);
    menuTrigger.setAttribute("aria-expanded", String(nextOpen));
    backdropButton.hidden = !nextOpen;

    if (restoreFocus) menuTrigger.focus();
  }

  menuTrigger.addEventListener("click", () => {
    setOpen(!sidebarRoot.classList.contains("open"));
  });

  backdropButton.addEventListener("click", () => setOpen(false, true));

  sidebarRoot.addEventListener("click", (event: MouseEvent) => {
    if (!mobile.matches) return;

    const target = event.target;
    if (!(target instanceof Element) || !target.closest("a[href]")) return;
    if (event.defaultPrevented) return;

    setOpen(false);
  });

  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "Escape" || !sidebarRoot.classList.contains("open"))
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

  function updateToggle(): void {
    if (!mobile.matches) {
      try {
        if (window.localStorage.getItem(SIDEBAR_HIDDEN_KEY) === "true")
          document.documentElement.classList.add("sidebar-hidden");
      } catch {
        // Storage can be unavailable in privacy-restricted browser contexts.
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
      // Storage can be unavailable in privacy-restricted browser contexts.
    }

    updateToggle();
  });

  mobile.addEventListener("change", updateToggle);
  updateToggle();
}

function initSidebarResize(): void {
  const sidebar = document.querySelector<HTMLElement>("[data-sidebar]");
  const handle = document.querySelector<HTMLElement>("[data-sidebar-resizer]");
  if (!sidebar || !handle) return;

  const sidebarRoot = sidebar;
  const resizeHandle = handle;
  const sidebarWidthURL = requiredAttribute(
    resizeHandle,
    "data-sidebar-width-url",
  );
  let startX = 0;
  let startWidth = 0;
  let activePointer: number | undefined;

  function setWidth(width: number): number {
    const next = syncSidebarWidthSetting(width);

    document.body.style.setProperty("--sidebar", `${next}px`);
    resizeHandle.setAttribute("aria-valuenow", String(next));
    return next;
  }

  async function persistWidth(width: number): Promise<void> {
    try {
      await postJSON(sidebarWidthURL, { width });
    } catch (error) {
      console.error("failed to save sidebar width", error);
    }
  }

  function finishResize(event?: PointerEvent): void {
    if (activePointer === undefined) return;
    if (event && event.pointerId !== activePointer) return;

    activePointer = undefined;
    document.body.classList.remove("sidebar-resizing");
    void persistWidth(Math.round(sidebarRoot.getBoundingClientRect().width));
  }

  handle.addEventListener("pointerdown", (event: PointerEvent) => {
    if (window.matchMedia("(max-width: 800px)").matches) return;

    activePointer = event.pointerId;
    startX = event.clientX;
    startWidth = sidebarRoot.getBoundingClientRect().width;
    handle.setPointerCapture(event.pointerId);
    document.body.classList.add("sidebar-resizing");
    event.preventDefault();
  });
  handle.addEventListener("pointermove", (event: PointerEvent) => {
    if (activePointer !== event.pointerId) return;
    setWidth(startWidth + event.clientX - startX);
  });
  handle.addEventListener("pointerup", finishResize);
  handle.addEventListener("pointercancel", finishResize);
  handle.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;

    event.preventDefault();

    const direction = event.key === "ArrowRight" ? 10 : -10;

    void persistWidth(
      setWidth(sidebarRoot.getBoundingClientRect().width + direction),
    );
  });
}

function initSidebarWidthSetting(): void {
  const input = document.querySelector<HTMLInputElement>(
    "[data-sidebar-width-setting]",
  );
  const presets = [
    ...document.querySelectorAll<HTMLButtonElement>(
      "[data-sidebar-width-preset]",
    ),
  ];
  if (!input || presets.length === 0) return;

  function apply(width: number): void {
    const next = syncSidebarWidthSetting(width);
    document.body.style.setProperty("--sidebar", `${next}px`);
  }

  for (const preset of presets) {
    preset.addEventListener("click", () => {
      apply(Number(preset.dataset.sidebarWidthPreset));
    });
  }

  syncSidebarWidthSetting(Number(input.value));
}

export function initLayout(): void {
  initMobileSidebar();
  initSidebarVisibility();
  initNavigationTree();
  initAccountMenus();
  initSidebarResize();
  initSidebarWidthSetting();
  for (const input of document.querySelectorAll<HTMLInputElement>(
    'input[name="navigation_style"]',
  )) {
    input.addEventListener("change", () => {
      if (!input.checked) return;
      document.body.dataset.navigationStyle = input.value;
      document.dispatchEvent(new Event("navigation-style-change"));
    });
  }
  const density = document.querySelector<HTMLSelectElement>(
    'select[name="navigation_density"]',
  );
  density?.addEventListener("change", () => {
    document.body.dataset.navigationDensity = density.value;
  });
  const typographySize = document.querySelector<HTMLSelectElement>(
    "[data-typography-size-setting]",
  );
  typographySize?.addEventListener("change", () => {
    document.body.dataset.typographySize =
      typographySize.value ||
      typographySize.dataset.defaultTypographySize ||
      "compact";
  });
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
