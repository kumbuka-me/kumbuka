import { expect, test } from "@playwright/test";

const base = process.env.E2E_OIDC_URL;

async function signIn(page, username) {
  await page.goto(base);
  await page.locator("#username").fill(username);
  await page.locator("#password").fill("local-test-password");
  await page.locator("#kc-login").click();
  await expect(page.locator("[data-account-menu-trigger]")).toBeVisible();
}

test.describe("OIDC authentication with Keycloak", () => {
  test("signs in an external administrator", async ({ page }) => {
    await signIn(page, "oidc-admin");

    await expect(page.locator("[data-account-menu-trigger] strong")).toHaveText(
      "OIDC Administrator",
    );
    await page.locator("[data-account-menu-trigger]").click();
    await expect(
      page.getByRole("menuitem", { name: "Administration" }),
    ).toBeVisible();
    expect((await page.request.get(new URL("admin", base).href)).status()).toBe(
      200,
    );

    const session = (await page.context().cookies()).find(
      (cookie) => cookie.name === "kumbuka_session",
    );
    expect(session).toMatchObject({ httpOnly: true, sameSite: "Lax" });
  });

  test("signs in an external viewer without administrator access", async ({
    page,
  }) => {
    await signIn(page, "oidc-viewer");

    await expect(page.locator("[data-account-menu-trigger] strong")).toHaveText(
      "OIDC Viewer",
    );
    await page.locator("[data-account-menu-trigger]").click();
    await expect(
      page.getByRole("menuitem", { name: "Administration" }),
    ).toHaveCount(0);
    expect((await page.request.get(new URL("admin", base).href)).status()).toBe(
      403,
    );
  });

  test("rejects callbacks without valid login state", async ({ request }) => {
    const response = await request.get(
      new URL("auth/callback?code=invalid&state=invalid", base).href,
    );
    expect(response.status()).toBe(400);
  });
});
