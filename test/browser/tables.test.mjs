import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { chromium } from "playwright";
import { pluginCatalog, pluginRoute } from "./plugin-fixture.mjs";

const module = {
  plugin_id: "me.kumbuka.tables",
  module_id: "interactive",
  name: "Tables",
  digest: "b".repeat(64),
};
const table = `<table class="kumbuka-table-sortable kumbuka-table-filterable kumbuka-table-styled"><thead><tr><th class="table-tone-blue">Service</th><th>Replicas</th></tr></thead><tbody><tr><td><a href="pages/db">DB</a></td><td>10</td></tr><tr><td>API</td><td>2</td></tr></tbody></table>`;

test("Tables uses packaged sorting/filtering, theme colors, and semantic fallback", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    let enabled = true;
    await page.route("http://tables.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (
        await pluginRoute(route, { enabled, module, assetDirectory: "tables" })
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
        body: `<style>:root{--text:#111111;--surface:#ffffff;--accent:#3366ff;--border:#cccccc;--surface-hover:#eeeeee;--surface-elevated:#dddddd;--text-secondary:#555555;--text-tertiary:#666666;--muted:#999999;}</style><body>${pluginCatalog(module, enabled)}<div data-kumbuka-plugin="me.kumbuka.tables" data-kumbuka-module="interactive" data-kumbuka-input="html"><div data-kumbuka-fallback>${table}</div></div><script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js';await renderPluginModules();</script></body>`,
      });
    });
    await page.goto("http://tables.test/");
    await page.locator("iframe[data-plugin-ready]").waitFor();
    const frame = page.frameLocator("iframe");
    assert.equal(await page.locator("[data-kumbuka-fallback]").isVisible(), false);
    await frame
      .getByRole("button", { name: "Sort by Replicas", exact: true })
      .click();
    assert.deepEqual(
      await frame.locator("tbody tr td:first-child").allTextContents(),
      ["API", "DB"],
    );
    await frame
      .getByRole("button", { name: "Filter Service", exact: true })
      .click();
    await frame.locator("input[type=search]").fill("api");
    assert.equal(await frame.locator("tbody tr:visible").count(), 1);
    assert.equal(
      await frame.locator("tbody tr:visible td:first-child").textContent(),
      "API",
    );
    const background = await frame
      .locator("th")
      .first()
      .evaluate((node) => getComputedStyle(node).backgroundColor);
    assert.notEqual(background, "rgba(0, 0, 0, 0)");
    assert.equal(
      await frame.locator("a").getAttribute("href"),
      "http://tables.test/pages/db",
    );
    enabled = false;
    await page.waitForTimeout(3200);
    assert.equal(await page.locator("iframe[data-plugin-ready]").count(), 1);
    await page.reload();
    assert.equal(await page.locator("[data-kumbuka-fallback]").isVisible(), true);
    assert.equal(
      await page.locator("[data-kumbuka-fallback] tbody tr").count(),
      2,
    );
    enabled = true;
    await page.reload();
    await page.locator("iframe[data-plugin-ready]").waitFor();
    assert.equal(await frame.locator("tbody tr:visible").count(), 2);
    await frame.getByRole("link", { name: "DB", exact: true }).click();
    await page.waitForURL("http://tables.test/pages/db");
  } finally {
    await browser.close();
  }
});

test("disabling a browser module cancels its pending image transfer", async () => {
  const browser = await chromium.launch({
    channel: process.env.BROWSER_CHANNEL || "chrome",
    headless: true,
  });
  try {
    const page = await browser.newPage();
    let enabled = true;
    let release;
    const held = new Promise((resolve) => {
      release = resolve;
    });
    const pixel = Buffer.from(
      "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+j7e0AAAAASUVORK5CYII=",
      "base64",
    );
    await page.route("http://transfer.test/**", async (route) => {
      const path = new URL(route.request().url()).pathname;
      if (
        await pluginRoute(route, { enabled, module, assetDirectory: "tables" })
      )
        return;
      if (path === "/image.png") {
        if (route.request().resourceType() === "fetch") await held;
        await route
          .fulfill({ contentType: "image/png", body: pixel })
          .catch(() => {});
        return;
      }
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
        body: `<body>${pluginCatalog(module, enabled)}<div data-kumbuka-plugin="me.kumbuka.tables" data-kumbuka-module="interactive" data-kumbuka-input="html"><div data-kumbuka-fallback><table class="kumbuka-table-sortable"><thead><tr><th>Image</th></tr></thead><tbody><tr><td><img src="/image.png"></td></tr></tbody></table></div></div><script type="module">import {renderPluginModules} from '/assets/js/plugins/loader.js';await renderPluginModules();</script></body>`,
      });
    });
    const transferring = page.waitForRequest(
      (request) =>
        request.url().endsWith("/image.png") &&
        request.resourceType() === "fetch",
    );
    await page.goto("http://transfer.test/", { waitUntil: "domcontentloaded" });
    await transferring;
    const cancelled = page.waitForEvent("requestfailed", {
      predicate: (request) =>
        request.url().endsWith("/image.png") &&
        request.resourceType() === "fetch",
    });
    enabled = false;
    await page.reload({ waitUntil: "domcontentloaded" });
    await cancelled;
    assert.equal(await page.locator("iframe").count(), 0);
    assert.equal(await page.locator("[data-kumbuka-fallback]").isVisible(), true);
    release();
  } finally {
    await browser.close();
  }
});

