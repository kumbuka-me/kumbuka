// Only core code runs in Kumbuka's document. Browser modules receive their own
// marked block's text inside a sandboxed frame and cannot return host HTML.
type Root = Document | HTMLElement;
type Module = {
  plugin_id: string;
  module_id: string;
  digest: string;
  name: string;
  frame_url: string;
};
type Active = {
  frame: HTMLIFrameElement;
  source: HTMLElement;
  version: string;
  finish: () => void;
  ready: Promise<void>;
  cleanup: () => void;
};
const active = new Map<HTMLElement, Active>();
let catalog: Module[] | undefined;
let watching = false;
const identifier = /^[a-z0-9][a-z0-9._-]{0,127}$/;
function validModule(value: unknown): value is Module {
  if (typeof value !== "object" || value === null) return false;
  const m = value as Record<string, unknown>;
  return (
    typeof m.plugin_id === "string" &&
    identifier.test(m.plugin_id) &&
    typeof m.module_id === "string" &&
    identifier.test(m.module_id) &&
    typeof m.digest === "string" &&
    /^[a-f0-9]{64}$/.test(m.digest) &&
    typeof m.name === "string" &&
    typeof m.frame_url === "string"
  );
}
function modules(): Module[] {
  if (catalog) return catalog;

  const source = document.getElementById("kumbuka-plugin-modules");
  if (!(source instanceof HTMLScriptElement)) return (catalog = []);

  let data: unknown;
  try {
    data = JSON.parse(source.textContent || "[]");
  } catch {
    return (catalog = []);
  }
  if (!Array.isArray(data) || !data.every(validModule)) return (catalog = []);

  const basePath = document.body.dataset.staticBasePath || "/";
  const pluginBase = new URL(
    basePath.replace(/\/?$/, "/") + "plugins/",
    location.origin,
  );
  catalog = data.filter((module) => {
    const frame = new URL(module.frame_url, location.href);
    return (
      frame.origin === location.origin &&
      frame.pathname.startsWith(pluginBase.pathname) &&
      !frame.search &&
      !frame.hash
    );
  });
  return catalog;
}

function remove(block: HTMLElement): void {
  const state = active.get(block);
  if (!state) return;
  state.cleanup();
  state.frame.remove();
  state.source.hidden = false;
  state.finish();
  active.delete(block);
}
function mount(block: HTMLElement, module: Module): Promise<void> {
  const htmlInput = block.dataset.kumbukaInput === "html";
  const source = block.querySelector<HTMLElement>(
    htmlInput ? ":scope > [data-kumbuka-fallback]" : ":scope > pre",
  );
  if (
    !source ||
    (htmlInput ? source.innerHTML.length : source.textContent?.length || 0) >
      1_000_000
  )
    return Promise.resolve();
  const transfer = new AbortController();
  const html = htmlInput
    ? prepareHTML(source, transfer.signal).catch(() => null)
    : Promise.resolve("");
  const frame = document.createElement("iframe");
  frame.className = "kumbuka-plugin-frame";
  frame.title = module.name;
  frame.setAttribute("sandbox", "allow-scripts");
  frame.setAttribute("referrerpolicy", "no-referrer");
  frame.setAttribute(
    "allow",
    "camera 'none'; microphone 'none'; geolocation 'none'; clipboard-read 'none'; clipboard-write 'none'",
  );
  frame.src = module.frame_url;
  const token = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
  let finish!: () => void;
  const ready = new Promise<void>((resolve) => {
    finish = resolve;
  });
  const timeout = window.setTimeout(() => remove(block), 15000);
  const message = async (event: MessageEvent<unknown>) => {
    if (
      event.source !== frame.contentWindow ||
      typeof event.data !== "object" ||
      event.data === null
    )
      return;
    const data = event.data as Record<string, unknown>;
    if (data.type === "kumbuka-plugin-listening") {
      const content = await html;
      if (active.get(block)?.frame !== frame) return;
      if (content === null) {
        remove(block);
        return;
      }
      frame.contentWindow?.postMessage(
        {
          type: "kumbuka-plugin-render",
          token,
          source: source.textContent || "",
          html: content,
          colors: themeColors(),
          theme: document.documentElement.style.colorScheme,
        },
        "*",
      );
      return;
    }
    if (data.token !== token) return;
    if (
      data.type === "kumbuka-plugin-link" &&
      typeof data.href === "string" &&
      navigator.userActivation.isActive
    ) {
      const links = [...source.querySelectorAll<HTMLAnchorElement>("a[href]")];
      const link = links.find((link) => link.href === data.href);
      if (link && /^https?:$/.test(new URL(link.href).protocol)) {
        if (link.target === "_blank")
          window.open(link.href, "_blank", "noopener,noreferrer");
        else location.assign(link.href);
      }
      return;
    }
    if (
      data.type === "kumbuka-plugin-ready" &&
      typeof data.height === "number" &&
      Number.isFinite(data.height)
    ) {
      frame.style.height = `${Math.min(10000, Math.max(24, data.height))}px`;
      source.hidden = true;
      frame.dataset.pluginReady = "true";
      clearTimeout(timeout);
      finish();
    } else if (data.type === "kumbuka-plugin-error") {
      remove(block);
    }
  };
  window.addEventListener("message", message);
  active.set(block, {
    frame,
    source,
    ready,
    finish,
    version: module.digest,
    cleanup: () => {
      transfer.abort();
      clearTimeout(timeout);
      window.removeEventListener("message", message);
    },
  });
  block.append(frame);
  return ready;
}
function watch(): void {
  if (watching) return;
  watching = true;
  new MutationObserver(() => {
    for (const block of active.keys()) if (!block.isConnected) remove(block);
  }).observe(document.body, { childList: true, subtree: true });
}
export async function renderPluginModules(
  root: Root = document,
): Promise<void> {
  const blocks = [
    ...root.querySelectorAll<HTMLElement>(
      "[data-kumbuka-plugin][data-kumbuka-module]",
    ),
  ];
  if (!blocks.length && !active.size) return;
  watch();
  const enabled = modules();
  const find = (block: HTMLElement) =>
    enabled.find(
      (m) =>
        m.plugin_id === block.dataset.kumbukaPlugin &&
        m.module_id === block.dataset.kumbukaModule,
    );
  for (const [block, state] of active)
    if (!block.isConnected || find(block)?.digest !== state.version)
      remove(block);
  await Promise.all(
    blocks.map((block) => {
      const module = find(block);
      if (!module) return Promise.resolve();
      return active.get(block)?.ready || mount(block, module);
    }),
  );
}

function themeColors(): Record<string, string> {
  const style = getComputedStyle(document.documentElement);
  const result: Record<string, string> = {};
  for (const name of [
    "text",
    "text-secondary",
    "text-tertiary",
    "surface",
    "surface-elevated",
    "surface-hover",
    "border",
    "border-strong",
    "border-subtle",
    "muted",
    "accent",
    "accent-secondary",
    "accent-soft",
    "success",
    "warning",
    "danger",
    "background",
  ]) {
    const value = style.getPropertyValue("--" + name).trim();
    if (CSS.supports("color", value)) result[name] = value;
  }
  return result;
}

// Normalize relative links before crossing document origins. Only same-origin
// raster image bytes are transferred; unavailable resources keep the native
// fallback visible instead of expanding the frame's network permissions.
async function prepareHTML(
  source: HTMLElement,
  signal: AbortSignal,
): Promise<string> {
  const copy = source.cloneNode(true) as HTMLElement;
  for (const link of copy.querySelectorAll<HTMLAnchorElement>("a[href]"))
    link.href = link.href;
  const images = copy.querySelectorAll<HTMLImageElement>("img");
  if (images.length > 32) throw new Error("Too many images");
  for (const image of images) {
    signal.throwIfAborted();
    const url = new URL(image.src, location.href);
    image.removeAttribute("srcset");
    if (url.protocol === "data:") continue;
    if (url.origin !== location.origin)
      throw new Error("Image unavailable in sandbox");
    const response = await fetch(url, {
      credentials: "same-origin",
      redirect: "error",
      signal: AbortSignal.any([signal, AbortSignal.timeout(5000)]),
    });
    if (!response.ok) throw new Error("Image unavailable");
    const blob = await rasterBlob(response);
    if (
      blob.size > 512000 ||
      !/^image\/(png|jpeg|gif|webp|avif|bmp)$/.test(blob.type)
    )
      throw new Error("Unsupported image");
    image.src = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result));
      reader.onerror = reject;
      reader.readAsDataURL(blob);
    });
  }
  const html = copy.innerHTML;
  if (html.length > 1_000_000) throw new Error("HTML input too large");
  return html;
}

async function rasterBlob(response: Response): Promise<Blob> {
  const type = response.headers.get("Content-Type")?.split(";", 1)[0] || "";
  if (!/^image\/(png|jpeg|gif|webp|avif|bmp)$/.test(type) || !response.body)
    throw new Error("Unsupported image");
  const reader = response.body.getReader();
  const parts: Uint8Array<ArrayBuffer>[] = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.length;
      if (size > 512000) throw new Error("Image too large");
      parts.push(new Uint8Array(value));
    }
  } finally {
    await reader.cancel();
    reader.releaseLock();
  }
  return new Blob(parts, { type });
}
