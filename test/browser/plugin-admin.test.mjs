import test from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { chromium } from "playwright";

function completed(child) {
  return new Promise((resolve, reject) => {
    let errors = "";
    child.stderr.on("data", (data) => (errors += data));
    child.on("error", reject);
    child.on("exit", (code) =>
      code === 0 ? resolve() : reject(new Error(errors)),
    );
  });
}
test("plugin administration forms drive the real runtime lifecycle", async () => {
  const directory = await mkdtemp(join(tmpdir(), "kumbuka-plugin-admin-"));
  let server, browser;
  try {
    const binary = join(directory, "server");
    await completed(
      spawn(
        "go",
        ["build", "-o", binary, "./test/browser/plugins-admin-server"],
        { stdio: ["ignore", "ignore", "pipe"] },
      ),
    );
    server = spawn(binary, [], { stdio: ["ignore", "pipe", "pipe"] });
    const url = await new Promise((resolve, reject) => {
      let output = "";
      const timeout = setTimeout(
        () => reject(new Error("server startup timeout")),
        30000,
      );
      server.on("error", reject);
      server.stdout.on("data", (chunk) => {
        output += chunk;
        const line = output.split("\n")[0];
        if (line.startsWith("http://") && output.includes("\n")) {
          clearTimeout(timeout);
          resolve(line);
        }
      });
      server.stderr.on("data", () => {});
    });
    browser = await chromium.launch({
      channel: process.env.BROWSER_CHANNEL || "chrome",
      headless: true,
    });
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(url + "/admin/plugins");
    assert.equal(
      await page.getByRole("heading", { name: "Plugins", level: 1, exact: true }).count(),
      1,
    );

    const tablesRow = page.locator(
      'tr[data-plugin-detail-open="me.kumbuka.tables"]',
    );
    await tablesRow.locator("td").nth(1).click();
    const tablesDialog = page.locator(
      '[data-plugin-detail-dialog][data-plugin-id="me.kumbuka.tables"]',
    );
    await tablesDialog.waitFor({ state: "visible" });
    assert.match(page.url(), /\?plugin=me\.kumbuka\.tables$/);

    await tablesDialog
      .getByRole("button", { name: "Disable plugin", exact: true })
      .click();
    await page.waitForURL(url + "/admin/plugins?plugin=me.kumbuka.tables");
    await tablesDialog
      .getByRole("button", { name: "Enable plugin", exact: true })
      .waitFor();
    assert.ok(
      !(
        await (await page.request.get(url + "/fixture/render")).text()
      ).includes("<table"),
    );

    await tablesDialog
      .getByRole("button", { name: "Enable plugin", exact: true })
      .click();
    await page.waitForURL(url + "/admin/plugins?plugin=me.kumbuka.tables");
    await tablesDialog
      .getByRole("button", { name: "Disable plugin", exact: true })
      .waitFor();
    assert.ok(
      (await (await page.request.get(url + "/fixture/render")).text()).includes(
        "<table",
      ),
    );
    await tablesDialog
      .getByRole("button", { name: "Close plugin details", exact: true })
      .click();
    await tablesDialog.waitFor({ state: "hidden" });
    await page.waitForURL(url + "/admin/plugins");

    const installForm = page.locator("[data-plugin-install]");
    const installButton = installForm.getByRole("button", {
      name: "Install and enable",
      exact: true,
    });
    assert.equal(await installButton.isDisabled(), true);
    assert.equal(
      await installForm.getByText("Drop plugin package here", { exact: true }).count(),
      1,
    );
    await installForm.locator("[data-plugin-upload-dropzone]").evaluate((dropzone) => {
      const transfer = new DataTransfer();
      transfer.items.add(
        new File(["fixture"], "dropped.kumbukaplugin", { type: "application/zip" }),
      );
      dropzone.dispatchEvent(
        new DragEvent("drop", {
          bubbles: true,
          cancelable: true,
          dataTransfer: transfer,
        }),
      );
    });
    assert.equal(
      await installForm.locator("[data-plugin-upload-name]").textContent(),
      "dropped.kumbukaplugin",
    );
    assert.equal(await installButton.isDisabled(), false);
    await installForm.locator("[data-plugin-upload-clear]").click();
    assert.equal(await installButton.isDisabled(), true);

    const upload = await page.request.get(url + "/fixture/package");
    await installForm.locator("[data-plugin-upload-input]").setInputFiles({
      name: "fixture.kumbukaplugin",
      mimeType: "application/zip",
      buffer: await upload.body(),
    });
    assert.equal(
      await installForm.locator("[data-plugin-upload-name]").textContent(),
      "fixture.kumbukaplugin",
    );
    assert.match(
      await installForm.locator("[data-plugin-upload-size]").textContent(),
      /(?:KiB|MiB)$/,
    );
    await installButton.click();
    await page.waitForURL(url + "/admin/plugins?plugin=io.example.browser");

    const browserDialog = page.locator(
      '[data-plugin-detail-dialog][data-plugin-id="io.example.browser"]',
    );
    await browserDialog.waitFor({ state: "visible" });
    assert.equal(
      await browserDialog
        .getByRole("heading", { name: "Browser Fixture", exact: true })
        .count(),
      1,
    );

    const upgradeForm = browserDialog.locator("[data-plugin-upgrade]");
    const upgradeDropzone = upgradeForm.locator("[data-plugin-upload-dropzone]");
    await upgradeDropzone.scrollIntoViewIfNeeded();
    const dropzoneBox = await upgradeDropzone.boundingBox();
    const dialogBodyBox = await browserDialog
      .locator(".plugin-detail-body")
      .boundingBox();
    assert.ok(dropzoneBox && dialogBodyBox);
    assert.ok(dropzoneBox.y >= dialogBodyBox.y);
    assert.ok(
      dropzoneBox.y + dropzoneBox.height <=
        dialogBodyBox.y + dialogBodyBox.height + 1,
    );

    const upgradeButton = upgradeForm.getByRole("button", {
      name: "Upgrade plugin",
      exact: true,
    });
    await upgradeForm.locator("[data-plugin-upload-input]").setInputFiles({
      name: "wrong.zip",
      mimeType: "application/zip",
      buffer: Buffer.from("invalid"),
    });
    await upgradeForm.getByRole("alert").waitFor();
    assert.equal(
      await upgradeForm.getByRole("alert").textContent(),
      "Choose a .kumbukaplugin package.",
    );
    assert.equal(await upgradeButton.isDisabled(), true);

    await upgradeForm.locator("[data-plugin-upload-input]").setInputFiles({
      name: "broken.kumbukaplugin",
      mimeType: "application/zip",
      buffer: Buffer.from("invalid"),
    });
    await upgradeButton.click();
    await browserDialog.getByRole("alert").waitFor();
    assert.match(
      await browserDialog.locator(".plugin-metadata").textContent(),
      /1\.0\.0/,
    );

    const upgrade = await page.request.get(
      url + "/fixture/package?version=1.1.0",
    );
    await browserDialog
      .locator("[data-plugin-upgrade] [data-plugin-upload-input]")
      .setInputFiles({
        name: "fixture.kumbukaplugin",
        mimeType: "application/zip",
        buffer: await upgrade.body(),
      });
    await browserDialog
      .locator("[data-plugin-upgrade]")
      .getByRole("button", { name: "Upgrade plugin", exact: true })
      .click();
    await page.waitForURL(url + "/admin/plugins?plugin=io.example.browser");
    await browserDialog.waitFor({ state: "visible" });
    assert.match(
      await browserDialog.locator(".plugin-metadata").textContent(),
      /1\.1\.0/,
    );

    await browserDialog
      .getByRole("button", { name: "Uninstall plugin", exact: true })
      .click();
    await page.locator("[data-confirm-dialog-accept]").click();
    await page.waitForURL(url + "/admin/plugins");
    assert.equal(
      await page
        .getByRole("link", { name: "Browser Fixture", exact: true })
        .count(),
      0,
    );
    const denied = await page.request.post(
      url + "/admin/plugins/me.kumbuka.tables/disable",
      { headers: { "X-Fixture-Role": "viewer" } },
    );
    assert.equal(denied.status(), 403);
    const crossSite = await page.request.post(
      url + "/admin/plugins/me.kumbuka.tables/disable",
      { headers: { "Sec-Fetch-Site": "cross-site" } },
    );
    assert.equal(crossSite.status(), 403);
    assert.deepEqual(errors, []);
    assert.ok(
      (await (await page.request.get(url + "/fixture/render")).text()).includes(
        "<table",
      ),
    );
  } finally {
    await browser?.close();
    server?.kill();
    await rm(directory, { recursive: true, force: true });
  }
});

