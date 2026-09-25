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
