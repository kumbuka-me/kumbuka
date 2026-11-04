import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import { pluginCatalog, pluginRoute } from "./plugin-fixture.mjs";

const browserOptions = {
  channel: process.env.BROWSER_CHANNEL || "chrome",
  headless: true,
};

async function serveAsset(route) {
  const path = new URL(route.request().url()).pathname;
  if (!path.startsWith("/assets/")) return false;
  const type = path.endsWith(".css")
    ? "text/css"
    : path.endsWith(".svg")
      ? "image/svg+xml"
      : "text/javascript";
  await route.fulfill({
    contentType: type,
    body: await readFile(new URL(`../../web/dist/${path.slice(8)}`, import.meta.url)),
  });
  return true;
}

test("ring indicator waits before appearing and cleans up on completion", async () => {
  const browser = await chromium.launch(browserOptions);
  try {
    const page = await browser.newPage();
    await page.route("http://loading.test/**", async (route) => {
      if (await serveAsset(route)) return;
      await route.fulfill({
        contentType: "text/html; charset=utf-8",
        body: `<link rel="stylesheet" href="/assets/css/app.css">
          <body><main id="loading"></main><script type="module">
          import { showDelayedLoading } from "/assets/js/core/loading.js";
          window.cancelQuick = showDelayedLoading(document.getElementById("loading"), "Fast task");
          window.cancelQuick();
          window.stopLoading = showDelayedLoading(document.getElementById("loading"), "Preparing diagram…");
          document.documentElement.dataset.ready = "true";
          </script></body>`,
      });
    });
    await page.goto("http://loading.test/");
    await page.locator("html[data-ready]").waitFor();
    assert.equal(await page.locator(".kumbuka-loading").count(), 0);
    await page.getByRole("status").filter({ hasText: "Preparing diagram…" }).waitFor();
    assert.equal(await page.getByText("Fast task").count(), 0);
    assert.match(
      await page.locator(".kumbuka-spinner").evaluate((element) =>
        getComputedStyle(element).animationName,
      ),
      /kumbuka-spinner-rotate/u,
    );
    await page.emulateMedia({ reducedMotion: "reduce" });
    assert.equal(
      await page.locator(".kumbuka-spinner").evaluate((element) =>
        getComputedStyle(element).animationName,
      ),
      "none",
    );
    await page.evaluate(() => window.stopLoading());
    assert.equal(await page.locator(".kumbuka-loading").count(), 0);
  } finally {
    await browser.close();
  }
});

test("Mermaid shows the host spinner while its sandbox is rendering", async () => {
  const browser = await chromium.launch(browserOptions);
  try {
    const page = await browser.newPage();
    await page.addInitScript(() => {
      window.addEventListener("message", (event) => {
        if (event.data === "mermaid-render-waiting")
          document.documentElement.dataset.mermaidWaiting = "true";
      });
    });
    const module = {
      plugin_id: "me.kumbuka.mermaid",
      module_id: "diagrams",
      name: "Mermaid",
      digest: "a".repeat(64),
    };
    const fakeJavaScript = `globalThis.kumbukaPlugin = {
      async render(root) {
        parent.postMessage("mermaid-render-waiting", "*");
        await new Promise(resolve => {
          const release = event => {
            if (event.data !== "mermaid-render-release") return;
            removeEventListener("message", release);
            resolve();
          };
          addEventListener("message", release);
        });
        root.textContent = "Rendered diagram";
      }
    };`;
    await page.route("http://mermaid-loading.test/**", async (route) => {
      if (await pluginRoute(route, { module, fakeJavaScript })) return;
      if (await serveAsset(route)) return;
      await route.fulfill({
        contentType: "text/html; charset=utf-8",
        body: `<link rel="stylesheet" href="/assets/css/app.css"><body>
          ${pluginCatalog(module)}
          <div class="kumbuka-plugin-block" data-kumbuka-plugin="me.kumbuka.mermaid" data-kumbuka-module="diagrams">
          <pre><code class="language-mermaid">flowchart TD\nA --> B</code></pre></div>
          <script type="module">
          import { renderPluginModules } from "/assets/js/plugins/loader.js";
          void renderPluginModules();
          </script></body>`,
      });
    });
    await page.goto("http://mermaid-loading.test/");
    await page.locator("html[data-mermaid-waiting]").waitFor();
    await page.getByRole("status").filter({ hasText: "Rendering diagram…" }).waitFor();
    assert.equal(await page.locator(".kumbuka-plugin-loading").count(), 1);
    assert.equal(await page.locator("pre").isVisible(), false);
    await page.locator("iframe").evaluate((frame) =>
      frame.contentWindow?.postMessage("mermaid-render-release", "*"),
    );
    await page.locator("iframe[data-plugin-ready]").waitFor();
    assert.equal(await page.locator(".kumbuka-loading").count(), 0);
    assert.equal(await page.locator(".kumbuka-plugin-loading").count(), 0);
    assert.equal(
      await page.frameLocator("iframe").getByText("Rendered diagram").count(),
      1,
    );
  } finally {
    await browser.close();
  }
});
