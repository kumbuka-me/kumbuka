import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import { block, plugin, pluginCatalog, pluginRoute } from "./plugin-fixture.mjs";

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
