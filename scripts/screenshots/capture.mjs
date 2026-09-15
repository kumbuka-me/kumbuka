import { mkdir } from "node:fs/promises";
import { chromium } from "playwright";

const baseURL = process.env.SCREENSHOT_BASE_URL;
const archive = process.env.SCREENSHOT_ARCHIVE;
const output = process.env.SCREENSHOT_OUTPUT;
const browserChannel = process.env.SCREENSHOT_BROWSER_CHANNEL || "chrome";

if (!baseURL || !archive || !output) {
  throw new Error("Missing screenshot environment configuration.");
}

await mkdir(output, { recursive: true });

const browser = await chromium.launch({ headless: true, channel: browserChannel });

try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 960 },
    deviceScaleFactor: 1,
    colorScheme: "light",
    reducedMotion: "reduce",
  });
  const page = await context.newPage();

  page.setDefaultTimeout(15_000);

  await page.goto(`${baseURL}/admin/import`, { waitUntil: "networkidle" });
  await page
    .locator('input[type="file"][name="files"]:not([webkitdirectory])')
    .setInputFiles(archive);
  await Promise.all([
    page.waitForURL(/\/admin\/import\?result=\d+$/),
    page.getByRole("button", { name: "Import pages" }).click(),
  ]);

  for (const path of [
    "/pages/getting-started",
    "/pages/content/editor",
    "/pages/knowledge/search",
  ]) {
    await page.goto(`${baseURL}${path}`, { waitUntil: "networkidle" });
  }

  await page.goto(`${baseURL}/`, { waitUntil: "networkidle" });
  await page.screenshot({ path: `${output}/dashboard.png` });

  await page.goto(`${baseURL}/edit/getting-started`, {
    waitUntil: "networkidle",
  });
  await page.getByRole("button", { name: "Split" }).click();
  await page.locator("[data-editor-workspace]").waitFor({ state: "visible" });
  await page.locator("[data-editor-preview-status]").waitFor({
    state: "hidden",
  });
  await page.locator("[data-editor-preview-content] > *").first().waitFor();
  await page.screenshot({ path: `${output}/editor.png` });
} finally {
  await browser.close();
}
