import { execFile } from "node:child_process";
import { fileURLToPath } from "node:url";

export const plugin = {
  plugin_id: "me.kumbuka.mermaid",
  module_id: "diagrams",
  name: "Mermaid",
  digest: "a".repeat(64),
};
export const base = `/plugins/${plugin.plugin_id}/${plugin.digest}/`;
export const block =
  '<div data-kumbuka-plugin="me.kumbuka.mermaid" data-kumbuka-module="diagrams"><pre><code class="language-mermaid">graph LR; A --> B</code></pre></div>';

const packageAssets = new Map();

export function pluginCatalog(module = plugin, enabled = true) {
  const modules = enabled
    ? [{ ...module, frame_url: `/plugins/${module.plugin_id}/${module.digest}/frames/${module.module_id}.html` }]
    : [];
  return `<script id="kumbuka-plugin-modules" type="application/json">${JSON.stringify(modules)}</script>`;
}

// readPluginAsset reads one browser asset from the pinned downloaded plugin package.
function readPluginAsset(pluginName, assetName) {
  const key = `${pluginName}/${assetName}`;
  const cached = packageAssets.get(key);
  if (cached) return cached;

  const packagePath = fileURLToPath(
    new URL(`../../plugins/${pluginName}.kumbukaplugin`, import.meta.url),
  );
  const pending = new Promise((resolve, reject) => {
    execFile(
      "unzip",
      ["-p", packagePath, `assets/${assetName}`],
      { encoding: null, maxBuffer: 32 * 1024 * 1024 },
      (error, stdout) => {
        if (error) {
          reject(
            new Error(
              `read ${pluginName} plugin asset ${assetName}: ${error.message}`,
              { cause: error },
            ),
          );
          return;
        }
        resolve(stdout);
      },
    );
  });
  packageAssets.set(key, pending);
  return pending;
}

// Browser fixture uses actual packaged JS/CSS and the core harness. HTTP tests
// independently verify the generated frame and response policy.
export async function pluginRoute(
  route,
  {
    enabled = true,
    fake = false,
    module = plugin,
    assetDirectory = "mermaid",
  } = {},
) {
  const base = `/plugins/${module.plugin_id}/${module.digest}/`;
  const framePath = base + "frames/" + module.module_id + ".html";
  const url = new URL(route.request().url());
  const path = url.pathname;
  if (!path.startsWith("/plugins/")) return false;
  if (!enabled) {
    await route.fulfill({ status: 404 });
    return true;
  }
  if (path === framePath) {
    const policy = `default-src 'none'; script-src ${url.origin}${base}assets/ ${url.origin}/plugins/runtime.js; style-src 'unsafe-inline' ${url.origin}${base}assets/; img-src data:; font-src data:; connect-src 'none'; sandbox allow-scripts`;
    await route.fulfill({
      contentType: "text/html",
      headers: { "Content-Security-Policy": policy },
      body: `<html><head><link rel="stylesheet" href="${base}assets/plugin.css"><script defer src="/plugins/runtime.js"></script></head><body data-plugin-javascript="${base}assets/plugin.js"><main id="plugin-root"></main></body></html>`,
    });
  } else if (path === "/plugins/runtime.js") {
    const { readFile } = await import("node:fs/promises");
    await route.fulfill({
      contentType: "text/javascript",
      body: await readFile(
        new URL("../../web/dist/js/plugins/frame.js", import.meta.url),
      ),
    });
  } else if (path === base + "assets/plugin.js" && fake) {
    await route.fulfill({
      contentType: "text/javascript",
      body: `globalThis.kumbukaPlugin={async render(root){await new Promise(r=>setTimeout(r,150));root.innerHTML='<svg height="80" aria-label="Diagram"></svg>';}};`,
    });
  } else if (path.startsWith(base + "assets/")) {
    const name = path.slice((base + "assets/").length);
    if (!["plugin.js", "plugin.css", "mermaid.min.js"].includes(name)) {
      await route.fulfill({ status: 404 });
      return true;
    }
    await route.fulfill({
      contentType: name.endsWith(".css") ? "text/css" : "text/javascript",
      body: await readPluginAsset(assetDirectory, name),
    });
  } else {
    await route.fulfill({ status: 404 });
  }
  return true;
}
