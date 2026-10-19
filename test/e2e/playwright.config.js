import { defineConfig } from "@playwright/test";

if (!process.env.E2E_PROXY_URL || !process.env.E2E_OIDC_URL) {
  throw new Error(
    "Set E2E_PROXY_URL and E2E_OIDC_URL to isolated test instances; never use production.",
  );
}

export default defineConfig({
  testDir: ".",
  timeout: 120_000,
  expect: { timeout: 10_000 },
  workers: 1,
  fullyParallel: false,
  outputDir: "../../test-results/e2e",
  reporter: process.env.CI ? "github" : "list",
  use: {
    browserName: "chromium",
    trace: "retain-on-failure",
    launchOptions: process.env.E2E_CHROMIUM_PATH
      ? { executablePath: process.env.E2E_CHROMIUM_PATH }
      : {},
  },
});
