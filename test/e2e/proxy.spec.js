import { expect, test } from "@playwright/test";
import { createServer, request as httpRequest } from "node:http";

const proxyURL = process.env.E2E_PROXY_URL;
const upstream = process.env.E2E_PROXY_UPSTREAM;
const untrusted = process.env.E2E_PROXY_UNTRUSTED_UPSTREAM;
const prefix = "/proxy-e2e/";
const base = new URL(prefix, proxyURL).href;

function identityProxy(target) {
  return createServer((client, response) => {
    const headers = { ...client.headers };
    for (const name of [
      "x-forwarded-user",
      "x-auth-request-user",
      "remote-user",
      "x-forwarded-email",
      "x-auth-request-email",
      "x-authentik-email",
      "x-forwarded-name",
      "x-auth-request-preferred-username",
      "x-authentik-name",
      "x-forwarded-groups",
      "x-auth-request-groups",
    ]) {
      delete headers[name];
    }

    const identity = /(?:^|;\s*)e2e_identity=(admin|viewer)(?:;|$)/.exec(
      headers.cookie || "",
    )?.[1];
    if (identity) {
      headers["x-forwarded-user"] = `proxy-${identity}`;
      headers["x-forwarded-email"] = `proxy-${identity}@example.test`;
      headers["x-forwarded-name"] =
        identity === "admin" ? "Proxy Administrator" : "Proxy Viewer";
      if (identity === "admin") {
        headers["x-forwarded-groups"] = "kumbuka-admins";
      }
    }

    const targetURL = new URL(client.url, target);
    const forwarded = httpRequest(
      targetURL,
      {
        method: client.method,
        headers: { ...headers, host: targetURL.host },
      },
      (result) => {
        response.writeHead(result.statusCode, result.headers);
        result.pipe(response);
      },
    );
    forwarded.on("error", () => {
      if (!response.headersSent) response.writeHead(502);
      response.end();
    });
    client.pipe(forwarded);
  });
}

test.describe("trusted-proxy authentication", () => {
  let proxy;

  test.beforeAll(async () => {
    if (!upstream || !untrusted) {
      throw new Error(
        "Set E2E_PROXY_UPSTREAM and E2E_PROXY_UNTRUSTED_UPSTREAM.",
      );
    }
    const address = new URL(proxyURL);
    if (address.hostname !== "127.0.0.1") {
      throw new Error("The test identity proxy must listen on 127.0.0.1.");
    }
    proxy = identityProxy(upstream);
    await new Promise((resolve, reject) => {
      proxy.once("error", reject);
      proxy.listen(Number(address.port), "127.0.0.1", resolve);
    });
  });

  test.afterAll(async () => {
    if (proxy) await new Promise((resolve) => proxy.close(resolve));
  });

  test("strips forged identity headers", async ({ request }) => {
    const response = await request.get(new URL("api/search", base).href, {
      headers: {
        "X-Forwarded-User": "forged-admin",
        "X-Forwarded-Groups": "kumbuka-admins",
      },
    });
    expect(response.status()).toBe(401);
  });

  test("grants the asserted administrator group", async ({ page }) => {
    await page
      .context()
      .addCookies([{ name: "e2e_identity", value: "admin", url: proxyURL }]);
    await page.goto(base);

    await expect(page.locator("[data-account-menu-trigger] strong")).toHaveText(
      "Proxy Administrator",
    );
    await page.locator("[data-account-menu-trigger]").click();
    await expect(
      page.getByRole("menuitem", { name: "Administration" }),
    ).toBeVisible();
    expect((await page.request.get(new URL("admin", base).href)).status()).toBe(
      200,
    );
  });

  test("keeps an asserted viewer out of administration", async ({ page }) => {
    await page
      .context()
      .addCookies([{ name: "e2e_identity", value: "viewer", url: proxyURL }]);
    await page.goto(base);

    await expect(page.locator("[data-account-menu-trigger] strong")).toHaveText(
      "Proxy Viewer",
    );
    await page.locator("[data-account-menu-trigger]").click();
    await expect(
      page.getByRole("menuitem", { name: "Administration" }),
    ).toHaveCount(0);
    expect((await page.request.get(new URL("admin", base).href)).status()).toBe(
      403,
    );
  });

  test("rejects identity headers from an untrusted peer", async ({
    request,
  }) => {
    const response = await request.get(
      new URL(`${prefix}api/search`, untrusted).href,
      {
        headers: {
          "X-Forwarded-User": "proxy-admin",
          "X-Forwarded-Groups": "kumbuka-admins",
        },
      },
    );
    expect(response.status()).toBe(401);
  });
});
