import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import {
  block,
  plugin,
  pluginCatalog,
  pluginRoute,
} from "./plugin-fixture.mjs";

test("plugin frames swap atomically and respect block presentation", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    const module = {
      plugin_id: "me.kumbuka.tasks",
      module_id: "task-ui",
      name: "Tasks",
      digest: "b".repeat(64),
    };
    await page.route("http://task.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (
        await pluginRoute(route, {
          module,
          fakeJavaScript: `globalThis.kumbukaPlugin={async render(root,context){const template=document.createElement('template');template.innerHTML=context.html;if(!template.content.querySelector('.kumbuka-task-list'))throw new Error('missing task list root');await new Promise(resolve=>setTimeout(resolve,150));root.textContent='Interactive task';}};`,
        })
      )
        return;
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          contentType: path.endsWith(".css") ? "text/css" : "text/javascript",
          body: await readFile(
            new URL("../../web/dist/" + path.slice(8), import.meta.url),
          ),
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<link rel="stylesheet" href="/assets/css/app.css"><style>.prose [data-kumbuka-fallback]{display:flex}</style><body><main class="prose">${pluginCatalog(module)}<span style="display:block" data-kumbuka-plugin="me.kumbuka.tasks" data-kumbuka-module="task-ui" data-kumbuka-input="html"><span class="kumbuka-task-list" data-kumbuka-fallback><span class="kumbuka-task-fallback">Task fallback</span></span></span></main><script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js';void renderPluginModules();</script></body>`,
      });
    });

    await page.goto("http://task.test/");
    const frame = page.locator("iframe");
    await frame.waitFor({ state: "attached" });
    assert.equal(await frame.evaluate((node) => getComputedStyle(node).visibility), "hidden");
    assert.equal(await page.getByText("Task fallback").isVisible(), true);
    await page.locator("iframe[data-plugin-ready]").waitFor();
    assert.equal(await frame.evaluate((node) => getComputedStyle(node).visibility), "visible");
    assert.equal(await page.getByText("Task fallback").isVisible(), false);
    assert.equal(await frame.evaluate((node) => node.classList.contains("kumbuka-plugin-frame-inline")), false);
  } finally {
    await browser.close();
  }
});

test("plugin frames include outer content margins in their measured height", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    const module = {
      plugin_id: "me.kumbuka.margin-test",
      module_id: "margin-ui",
      name: "Margin Test",
      digest: "d".repeat(64),
    };
    const fakeJavaScript = `
      globalThis.kumbukaPlugin = {
        render(root) {
          const content = document.createElement("div");
          content.textContent = "Measured content";
          content.style.height = "40px";
          content.style.margin = "32px 0";
          root.append(content);
        }
      };
    `;

    await page.route("http://margin.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (
        await pluginRoute(route, {
          module,
          fakeJavaScript,
        })
      )
        return;
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          contentType: "text/javascript",
          body: await readFile(
            new URL("../../web/dist/" + path.slice(8), import.meta.url),
          ),
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<body>${pluginCatalog(module)}<div data-kumbuka-plugin="me.kumbuka.margin-test" data-kumbuka-module="margin-ui"><pre>margin test</pre></div><script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js';await renderPluginModules();</script></body>`,
      });
    });

    await page.goto("http://margin.test/");
    await page.locator("iframe[data-plugin-ready]").waitFor();

    const frame = page.frames().find((candidate) => candidate !== page.mainFrame());
    assert.ok(frame);
    const dimensions = await frame.evaluate(() => ({
      clientHeight: document.documentElement.clientHeight,
      scrollHeight: document.documentElement.scrollHeight,
      rootHeight: document.getElementById("plugin-root")?.getBoundingClientRect()
        .height,
    }));

    assert.equal(dimensions.scrollHeight, dimensions.clientHeight);
    assert.equal(dimensions.rootHeight, 104);
  } finally {
    await browser.close();
  }
});

test("real Mermaid is isolated and plugin changes require a page reload", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    let enabled = true;
    let assetRequests = 0;
    await page.route("http://plugins.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (path.includes("/assets/plugin.js")) assetRequests++;
      if (await pluginRoute(route, { enabled })) return;
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          contentType: "text/javascript",
          body: await readFile(
            new URL("../../web/dist/" + path.slice(8), import.meta.url),
          ),
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<body>${pluginCatalog(plugin, enabled)}${block}<script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js'; await renderPluginModules();</script></body>`,
      });
    });
    await page.goto("http://plugins.test/");
    await page.locator("iframe[data-plugin-ready]").waitFor();
    assert.equal(await page.frameLocator("iframe").locator("svg").count(), 1);
    const frame = page.frames().find((f) => f !== page.mainFrame());
    assert.equal(
      await frame.evaluate(() => {
        try {
          return parent.document.body ? "accessible" : "missing";
        } catch {
          return "blocked";
        }
      }),
      "blocked",
    );
    assert.equal(
      await frame.evaluate(async () => {
        try {
          await fetch("/private");
          return "accessible";
        } catch {
          return "blocked";
        }
      }),
      "blocked",
    );
    assert.equal(
      await page.locator("iframe").getAttribute("sandbox"),
      "allow-scripts",
    );
    assert.equal(
      await page.locator("iframe").getAttribute("allow"),
      "camera 'none'; microphone 'none'; geolocation 'none'",
    );
    enabled = false;
    const stopped = assetRequests;
    await page.waitForTimeout(3200);
    assert.equal(await page.locator("iframe[data-plugin-ready]").count(), 1);
    assert.equal(assetRequests, stopped);

    await page.reload();
    assert.equal(await page.locator("iframe").count(), 0);
    assert.equal(await page.locator("pre").isVisible(), true);

    enabled = true;
    await page.reload();
    await page.locator("iframe[data-plugin-ready]").waitFor();
    assert.ok(assetRequests > stopped);
    assert.equal(await page.frameLocator("iframe").locator("svg").count(), 1);
  } finally {
    await browser.close();
  }
});

test("trusted browser-module changes relay only same-plugin widget commands", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    const statusModule = {
      plugin_id: "me.kumbuka.status-dropdowns",
      module_id: "status-ui",
      name: "Status Dropdowns",
      digest: "c".repeat(64),
      commands: [{ module_id: "page-details", surface: "page.details" }],
    };
    const fakeJavaScript = `
      globalThis.kumbukaPlugin = {
        render(root) {
          const select = document.createElement("select");
          select.setAttribute("aria-label", "API status");
          select.size = 2;
          select.dataset.kumbukaCommandModule = "page-details";
          const todo = document.createElement("option");
          todo.value = "set-aaaaaaaaaaaaaaaaaaaaaaaa-0";
          todo.textContent = "To do";
          const done = document.createElement("option");
          done.value = "set-aaaaaaaaaaaaaaaaaaaaaaaa-3";
          done.textContent = "Done";
          select.append(todo, done);
          root.append(select);
        }
      };
    `;
    let commandRequest;
    let documentRequests = 0;

    await page.route("http://status.test/**", async (route) => {
      const request = route.request();
      if (request.resourceType() === "document") documentRequests++;
      const path = new URL(request.url()).pathname;
      if (
        path ===
        "/plugins/actions/me.kumbuka.status-dropdowns/page-details/set-aaaaaaaaaaaaaaaaaaaaaaaa-3"
      ) {
        commandRequest = request;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({}),
        });
        return;
      }
      if (
        await pluginRoute(route, {
          enabled: true,
          fakeJavaScript,
          module: statusModule,
        })
      )
        return;
      if (path.startsWith("/assets/")) {
        await route.fulfill({
          contentType: "text/javascript",
          body: await readFile(
            new URL("../../web/dist/" + path.slice(8), import.meta.url),
          ),
        });
        return;
      }
      await route.fulfill({
        contentType: "text/html",
        body: `<body data-current-page="release/readiness">${pluginCatalog(statusModule, true)}<span data-kumbuka-plugin="me.kumbuka.status-dropdowns" data-kumbuka-module="status-ui" data-kumbuka-input="html"><span data-kumbuka-fallback>In progress</span></span><script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js';await renderPluginModules();</script></body>`,
      });
    });

    await page.goto("http://status.test/pages/release/readiness");
    await page.locator("iframe[data-plugin-ready]").waitFor();

    const documentRequestsBeforeCommand = documentRequests;
    const command = page.waitForRequest(
      (request) =>
        request.method() === "POST" &&
        new URL(request.url()).pathname.endsWith(
          "/set-aaaaaaaaaaaaaaaaaaaaaaaa-3",
        ),
    );
    // Click a visible option to produce a trusted native change on every platform.
    await page
      .frameLocator("iframe")
      .getByRole("option", { name: "Done", exact: true })
      .click();
    await command;
    await page.waitForTimeout(50);

    assert.ok(commandRequest);
    assert.equal(commandRequest.resourceType(), "fetch");
    const form = new URLSearchParams(commandRequest.postData() || "");
    assert.equal(form.get("surface"), "page.details");
    assert.equal(form.get("page"), "release/readiness");
    assert.equal(form.get("next"), "/pages/release/readiness");
    assert.equal(form.get("response"), "json");
    assert.equal(documentRequests, documentRequestsBeforeCommand);
    assert.equal(page.url(), "http://status.test/pages/release/readiness");
  } finally {
    await browser.close();
  }
});
