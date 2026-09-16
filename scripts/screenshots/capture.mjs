import { mkdir } from "node:fs/promises";
import { chromium } from "playwright";

const baseURL = process.env.SCREENSHOT_BASE_URL;
const archive = process.env.SCREENSHOT_ARCHIVE;
const output = process.env.SCREENSHOT_OUTPUT;
const editorSlug = process.env.SCREENSHOT_EDITOR_SLUG;
const browserChannel = (process.env.SCREENSHOT_BROWSER_CHANNEL || "").trim();
const visitPaths = (process.env.SCREENSHOT_VISITS || "")
  .split(",")
  .map((value) => value.trim())
  .filter(Boolean);

if (!baseURL || !archive || !output || !editorSlug) {
  throw new Error("Missing screenshot environment configuration.");
}

for (const path of visitPaths) {
  if (!path.startsWith("/")) {
    throw new Error(`Screenshot visit path must be absolute: ${path}`);
  }
}

await mkdir(output, { recursive: true });

const launchOptions = { headless: true };
if (browserChannel) launchOptions.channel = browserChannel;

const browser = await chromium.launch(launchOptions);

try {
  const context = await browser.newContext({
    viewport: { width: 1440, height: 960 },
    deviceScaleFactor: 1,
    colorScheme: "light",
    reducedMotion: "reduce",
    serviceWorkers: "block",
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

  for (const path of visitPaths) {
    await page.goto(new URL(path, baseURL).toString(), { waitUntil: "networkidle" });
  }

  await page.goto(`${baseURL}/`, { waitUntil: "networkidle" });
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({
    path: `${output}/dashboard.png`,
    animations: "disabled",
  });

  await page.goto(`${baseURL}/edit/${editorSlug}`, {
    waitUntil: "networkidle",
  });
  await page.getByRole("button", { name: "Split" }).click();
  await page.locator("[data-editor-workspace]").waitFor({ state: "visible" });
  await page.locator("[data-editor-preview-status]").waitFor({
    state: "hidden",
  });
  await page.locator("[data-editor-preview-content] > *").first().waitFor();
  await page.evaluate(() => document.fonts.ready);
  await page.screenshot({
    path: `${output}/editor.png`,
    animations: "disabled",
  });
} finally {
  await browser.close();
}
