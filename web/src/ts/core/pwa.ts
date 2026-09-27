import { route, routePrefix } from "./route.ts";
// Service worker setup and private-cache protection.

function pageCachePrefix(): string {
  return routePrefix()
    ? `kumbuka-scope-${encodeURIComponent(route("/"))}-pages-`
    : "kumbuka-pages-";
}

function configureUserMessage(userID: string): ConfigureUserMessage {
  return { type: "configure-user", userID };
}

// Clears private page caches.
async function clearPrivatePageCaches(): Promise<void> {
  if (!("caches" in window)) return;

  const names = await caches.keys();

  await Promise.all(
    names
      .filter((name) => name.startsWith(pageCachePrefix()))
      .map((name) => caches.delete(name)),
  );
}

// Configures worker.
function configureWorker(registration: ServiceWorkerRegistration): void {
  const userID = document.body?.dataset.userId || "";
  const worker =
    registration.active || registration.waiting || registration.installing;

  worker?.postMessage(configureUserMessage(userID));
}

// Protects logout.
function protectLogout(): void {
  const form = document.querySelector<HTMLFormElement>(
    `form[action="${route("/auth/logout")}"]`,
  );
  if (!form || !("caches" in window)) return;

  form.addEventListener("submit", async (event: SubmitEvent) => {
    if (form.dataset.pwaLogout === "true") return;

    event.preventDefault();

    try {
      const registration = await navigator.serviceWorker?.getRegistration(
        route("/"),
      );
      if (registration && new URL(registration.scope).pathname === route("/")) {
        const worker =
          registration.active ||
          registration.waiting ||
          registration.installing;
        worker?.postMessage(configureUserMessage(""));
      }
      await clearPrivatePageCaches();
    } catch {
      /* best effort */
    }

    form.dataset.pwaLogout = "true";
    form.submit();
  });
}

function canRegisterServiceWorker(): boolean {
  if (!("serviceWorker" in navigator)) return false;
  if (location.protocol !== "http:") return true;

  return location.hostname === "localhost" || location.hostname === "127.0.0.1";
}

// Reuses the existing deployment registration so normal page loads do not fetch sw.js again.
export async function ensureServiceWorkerRegistration(
  container: ServiceWorkerContainer,
): Promise<ServiceWorkerRegistration> {
  const existing = await container.getRegistration(route("/"));
  if (existing && new URL(existing.scope).pathname === route("/"))
    return existing;

  return container.register(route("/sw.js"), { scope: route("/") });
}

// Initializes pwa.
export function initPWA(): void {
  protectLogout();
  if (!canRegisterServiceWorker()) return;

  async function registerServiceWorker(): Promise<void> {
    try {
      const registration = await ensureServiceWorkerRegistration(
        navigator.serviceWorker,
      );
      configureWorker(registration);
    } catch {
      // Offline support is best effort and must never interfere with normal navigation.
    }
  }

  window.addEventListener("load", () => void registerServiceWorker());
}
