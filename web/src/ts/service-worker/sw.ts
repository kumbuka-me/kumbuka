/// <reference path="../contracts.d.ts" />
/// <reference lib="webworker" />

// Offline asset caching and user-scoped private page caching.

const assetCacheName = "kumbuka-assets-v1";
const pageCachePrefix = "kumbuka-pages-v2-";
let pageCacheName = "";
const clientCaches = new Map<string, string>();
let generation = 0;
let privateOperations: Promise<unknown> = Promise.resolve();

// Serialize writes and deletion so logout cannot be undone by an in-flight put.
function privateOperation(work: () => Promise<unknown>): Promise<void> {
  const result = privateOperations.then(work).then(() => undefined);
  privateOperations = result.catch(() => undefined);
  return result;
}

const serviceWorker = self as unknown as ServiceWorkerGlobalScope;

function isConfigureUserMessage(value: unknown): value is ConfigureUserMessage {
  if (typeof value !== "object" || value === null) return false;

  const message = value as { type?: unknown; userID?: unknown };
  return (
    message.type === "configure-user" && typeof message.userID === "string"
  );
}

function handleInstall(): void {
  void serviceWorker.skipWaiting();
}

function staleCache(name: string): boolean {
  return (
    name.startsWith("kumbuka-offline-") ||
    (name.startsWith("kumbuka-pages-") && !name.startsWith(pageCachePrefix)) ||
    (name.startsWith("kumbuka-assets-") && name !== assetCacheName)
  );
}

async function activateServiceWorker(): Promise<void> {
  const names = await caches.keys();

  await Promise.all(
    names.filter(staleCache).map((name) => caches.delete(name)),
  );
  await serviceWorker.clients.claim();
}

function handleActivate(event: ExtendableEvent): void {
  event.waitUntil(activateServiceWorker());
}

async function removeOtherPrivateCaches(): Promise<void> {
  await privateOperation(async () => {
    const names = await caches.keys();

    await Promise.all(
      names
        .filter(
          (name) => name.startsWith(pageCachePrefix) && name !== pageCacheName,
        )
        .map((name) => caches.delete(name)),
    );
  });
}

function handleMessage(event: ExtendableMessageEvent): void {
  const data: unknown = event.data;
  if (!isConfigureUserMessage(data)) return;

  const source = event.source;
  if (!source || !("id" in source)) return;

  const nextCache = /^[1-9][0-9]*$/.test(data.userID)
    ? `${pageCachePrefix}${data.userID}`
    : "";

  if (nextCache !== pageCacheName || !nextCache) {
    generation++;
    clientCaches.clear();
    pageCacheName = nextCache;
  }
  if (nextCache) clientCaches.set(source.id, nextCache);

  event.waitUntil(removeOtherPrivateCaches());
}

function cacheablePage(url: URL): boolean {
  return (
    url.origin === serviceWorker.location.origin &&
    (url.pathname === "/" || url.pathname.startsWith("/pages/"))
  );
}

async function fetchAsset(request: Request): Promise<Response> {
  const cache = await caches.open(assetCacheName);
  const cached = await cache.match(request);
  if (cached) return cached;

  try {
    const response = await fetch(request);
    if (response.ok && !response.redirected) {
      try {
        await cache.put(request, response.clone());
      } catch {
        // A failed cache write must not fail the network response.
      }
    }
    return response;
  } catch {
    return Response.error();
  }
}

function privateCacheIsCurrent(
  requestCache: string,
  requestGeneration: number,
): boolean {
  return requestGeneration === generation && requestCache === pageCacheName;
}

function shouldCachePrivateResponse(
  response: Response,
  responseCache: string,
  requestCache: string,
  requestGeneration: number,
): boolean {
  if (!response.ok || response.redirected) return false;
  if (responseCache !== requestCache) return false;

  return privateCacheIsCurrent(requestCache, requestGeneration);
}

async function cachePrivateResponse(
  request: Request,
  response: Response,
  requestCache: string,
  requestGeneration: number,
): Promise<void> {
  await privateOperation(async () => {
    if (!privateCacheIsCurrent(requestCache, requestGeneration)) return;

    const cache = await caches.open(requestCache);
    await cache.put(request, response.clone());
  }).catch(() => undefined);
}

async function cachedPrivateResponse(
  request: Request,
  requestCache: string,
  requestGeneration: number,
): Promise<Response> {
  if (!privateCacheIsCurrent(requestCache, requestGeneration))
    return Response.error();

  const cache = await caches.open(requestCache);
  const cached = await cache.match(request);

  if (!privateCacheIsCurrent(requestCache, requestGeneration))
    return Response.error();

  return cached || Response.error();
}

async function fetchPrivatePage(
  request: Request,
  requestCache: string,
  requestGeneration: number,
): Promise<Response> {
  try {
    const response = await fetch(request);

    // Redirected login pages and responses for a different signed-in user
    // must never enter the requesting tab's private cache.
    const responseCache = `${pageCachePrefix}${response.headers.get("X-Kumbuka-User-ID") || ""}`;
    if (
      shouldCachePrivateResponse(
        response,
        responseCache,
        requestCache,
        requestGeneration,
      )
    ) {
      await cachePrivateResponse(
        request,
        response,
        requestCache,
        requestGeneration,
      );
    }

    return response;
  } catch {
    return cachedPrivateResponse(request, requestCache, requestGeneration);
  }
}

function handleFetch(event: FetchEvent): void {
  const request = event.request;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== serviceWorker.location.origin) return;

  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(fetchAsset(request));
    return;
  }

  const requestCache = clientCaches.get(event.clientId);
  if (!requestCache || !cacheablePage(url)) return;

  event.respondWith(fetchPrivatePage(request, requestCache, generation));
}

serviceWorker.addEventListener("install", handleInstall);
serviceWorker.addEventListener("activate", handleActivate);
serviceWorker.addEventListener("message", handleMessage);
serviceWorker.addEventListener("fetch", handleFetch);
