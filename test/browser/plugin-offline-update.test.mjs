import test from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { once } from "node:events";
import { mkdtemp, mkdir, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { chromium } from "playwright";

test("offline update submits the embedded candidate with JavaScript enabled", { timeout: 90000 }, async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "kumbuka-offline-update-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const binary = join(directory, "server");
  const build = spawn("go", ["build", "-o", binary, "./test/browser/plugins-admin-server"], { stdio: "inherit" });
  const [buildCode] = await once(build, "exit");
  assert.equal(buildCode, 0);
  const server = spawn(binary, [], {
    env: { ...process.env, KUMBUKA_FIXTURE_OFFLINE_UPDATE: "1" },
    stdio: ["ignore", "pipe", "pipe"],
  });
  t.after(() => server.kill());
  let diagnostics = "";
  server.stderr.on("data", (chunk) => { diagnostics += chunk; });
  const url = await new Promise((resolve, reject) => {
    let output = "";
    const timeout = setTimeout(() => reject(new Error(`startup timeout: ${diagnostics}`)), 30000);
    server.on("error", reject);
    server.on("exit", () => { clearTimeout(timeout); reject(new Error(diagnostics || "server exited")); });
    server.stdout.on("data", (chunk) => {
      output += chunk;
      if (output.startsWith("http://") && output.includes("\n")) {
        clearTimeout(timeout);
        resolve(output.split("\n")[0]);
      }
    });
  });
  const browser = await chromium.launch({ channel: process.env.BROWSER_CHANNEL || "chrome", headless: true });
  t.after(() => browser.close());
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 } });
  const errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  await page.goto(url + "/admin/plugins?plugin=io.example.browser");
  const dialog = page.locator('[data-plugin-detail-dialog][data-plugin-id="io.example.browser"]');
  const button = dialog.getByRole("button", { name: "Update offline to 1.1.0", exact: true });
  await button.scrollIntoViewIfNeeded();
  const screenshotDirectory = process.env.KUMBUKA_SCREENSHOT_DIR;
  if (screenshotDirectory) {
    await mkdir(screenshotDirectory, { recursive: true });
    await page.screenshot({ path: join(screenshotDirectory, "plugin-offline-update-available.png") });
  }
  const requestPromise = page.waitForRequest((request) => request.method() === "POST" && request.url().includes("/io.example.browser/update"));
  await Promise.all([page.waitForNavigation({ waitUntil: "networkidle" }), button.click()]);
  const request = await requestPromise;
  assert.match(request.postData() ?? "", /(?:source=builtin|name="source"\r?\n\r?\nbuiltin)/);
  await page.waitForURL(url + "/admin/plugins?plugin=io.example.browser");
  await button.waitFor({ state: "detached" });
  assert.match(await dialog.innerText(), /1\.1\.0/);
  assert.equal(await dialog.getByRole("button", { name: /Update offline/ }).count(), 0);
  if (screenshotDirectory) {
    await dialog.getByRole("heading", { name: "Updates", exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: join(screenshotDirectory, "plugin-offline-update-complete.png") });
  }
  assert.deepEqual(errors, []);
});
