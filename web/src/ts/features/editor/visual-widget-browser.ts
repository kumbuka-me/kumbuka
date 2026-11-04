// Sandboxed editor integration for plugin-owned visual widget controls.
import type { CatalogWidget, CatalogWidgetSetting } from "./widget-contract.ts";
import { route } from "../../core/route.ts";

type EditorBrowserModule = {
  plugin_id: string;
  module_id: string;
  frame_url: string;
};

// editorBrowserFrameURL resolves only a loaded browser module owned by this widget's plugin.
function editorBrowserFrameURL(
  widget: CatalogWidget,
  moduleID: string,
): string | null {
  const source = document.getElementById("kumbuka-plugin-modules");
  if (!source) return null;
  let modules: unknown;
  try {
    modules = JSON.parse(source.textContent || "[]");
  } catch {
    return null;
  }
  if (!Array.isArray(modules)) return null;
  const module = modules.find(
    (item): item is EditorBrowserModule =>
      typeof item === "object" &&
      item !== null &&
      item.plugin_id === widget.plugin_id &&
      item.module_id === moduleID &&
      typeof item.frame_url === "string",
  );
  if (!module) return null;
  let url: URL;
  try {
    url = new URL(module.frame_url, location.href);
  } catch {
    return null;
  }
  const prefix = new URL(route("/plugins/"), location.href);
  if (
    url.origin !== location.origin ||
    !url.pathname.startsWith(prefix.pathname) ||
    url.search ||
    url.hash
  )
    return null;
  return url.href;
}

// createBrowserSetting mounts an isolated plugin-provided editor and stores only bounded source text.
export function createBrowserSetting(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): HTMLElement {
  const wrapper = document.createElement("div");
  wrapper.className = "visual-widget-field visual-widget-browser";
  const title = document.createElement("span");
  title.className = "visual-widget-field-label";
  title.textContent = setting.label;
  wrapper.append(title);

  const attribute = setting.attribute || "";
  const control = document.createElement("textarea");
  control.dataset.widgetAttribute = attribute;
  control.value = values[attribute] || "";
  control.rows = 9;
  control.setAttribute("aria-label", `${setting.label} source`);
  wrapper.append(control);
  const frameURL = editorBrowserFrameURL(
    widget,
    setting.browser_module_id || "",
  );
  if (!frameURL) return wrapper;

  control.hidden = true;
  const frame = document.createElement("iframe");
  frame.className = "kumbuka-plugin-frame visual-widget-browser-frame";
  frame.title = `${widget.name} editor`;
  frame.setAttribute("sandbox", "allow-scripts");
  frame.setAttribute("referrerpolicy", "no-referrer");
  frame.setAttribute(
    "allow",
    "camera 'none'; microphone 'none'; geolocation 'none'",
  );
  frame.style.width = "100%";
  frame.style.minHeight = "180px";
  frame.src = frameURL;
  const token = Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
    byte.toString(16).padStart(2, "0"),
  ).join("");
  const onMessage = (event: MessageEvent<unknown>) => {
    if (
      event.source !== frame.contentWindow ||
      !event.data ||
      typeof event.data !== "object"
    )
      return;
    const data = event.data as Record<string, unknown>;
    if (data.type === "kumbuka-plugin-listening") {
      frame.contentWindow?.postMessage(
        {
          type: "kumbuka-plugin-edit",
          source: control.value,
          token,
          theme:
            document.documentElement.dataset.theme === "dark"
              ? "dark"
              : "light",
        },
        "*",
      );
      return;
    }
    if (data.token !== token) return;
    if (
      data.type === "kumbuka-plugin-editor-change" &&
      typeof data.source === "string" &&
      data.source.length <= 65536
    ) {
      control.value = data.source;
      control.dispatchEvent(new Event("input", { bubbles: true }));
    }
    if (
      data.type === "kumbuka-plugin-ready" &&
      typeof data.height === "number" &&
      Number.isFinite(data.height)
    ) {
      frame.style.height = `${Math.max(180, Math.min(10000, data.height))}px`;
    }
    if (data.type === "kumbuka-plugin-error") control.hidden = false;
  };
  window.addEventListener("message", onMessage);
  const observer = new MutationObserver(() => {
    if (!wrapper.isConnected) {
      window.removeEventListener("message", onMessage);
      observer.disconnect();
    }
  });
  observer.observe(document.body, { childList: true, subtree: true });
  wrapper.append(frame);
  return wrapper;
}
