// Restores the sidebar before first paint after a sidebar page navigation.

(() => {
  const navigationScrollKeyPrefix = "kumbuka:navigation-scroll";
  const mode = window.matchMedia("(max-width: 800px)").matches
    ? "mobile"
    : "desktop";
  const scrollKey = `${navigationScrollKeyPrefix}:${mode}`;
  let savedScrollTop: number | undefined;

  if (mode === "desktop") {
    try {
      if (window.localStorage.getItem("kumbuka:sidebar-hidden") === "true")
        document.documentElement.classList.add("sidebar-hidden");
    } catch {
      // Storage can be unavailable in privacy-restricted browser contexts.
    }
  }

  try {
    const saved = window.sessionStorage.getItem(scrollKey);
    if (saved !== null) {
      window.sessionStorage.removeItem(scrollKey);

      const state = JSON.parse(saved) as {
        destination?: unknown;
        scrollTop?: unknown;
      };
      const destination = `${window.location.pathname}${window.location.search}`;

      const scrollTop = state.scrollTop;
      const sameDestination = state.destination === destination;
      const validScrollTop =
        typeof scrollTop === "number" && Number.isFinite(scrollTop);
      if (sameDestination && validScrollTop) savedScrollTop = scrollTop;
    }
  } catch {
    // Storage can be unavailable in privacy-restricted browser contexts.
  }

  function prepareNavigation(): boolean {
    const ready = document.querySelector<HTMLElement>(
      "[data-navigation-scroll-ready]",
    );
    if (!ready) return false;

    const navigation = ready.parentElement?.querySelector<HTMLElement>(
      ":scope > nav[data-navigation]",
    );
    if (!navigation) return true;

    if (savedScrollTop !== undefined) {
      navigation.scrollTop = savedScrollTop;
    } else {
      const active = navigation.querySelector<HTMLElement>(
        '[aria-current="page"]',
      );
      if (active) {
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

    navigation.dataset.navigationScrollRestored = "true";
    return true;
  }

  if (prepareNavigation()) return;

  const observer = new MutationObserver(() => {
    if (!prepareNavigation()) return;
    observer.disconnect();
  });

  observer.observe(document.documentElement, {
    childList: true,
    subtree: true,
  });
  document.addEventListener(
    "DOMContentLoaded",
    () => {
      prepareNavigation();
      observer.disconnect();
    },
    { once: true },
  );
})();
