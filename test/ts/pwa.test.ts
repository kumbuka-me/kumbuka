import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

import { ensureServiceWorkerRegistration } from "../../web/src/ts/core/pwa.ts";

void test("service worker registration reuses an existing root registration", async () => {
  const registration = {
    scope: "https://wiki.example/",
  } as ServiceWorkerRegistration;
  let registerCalls = 0;
  const container = {
    getRegistration: async () => registration,
    register: async () => {
      registerCalls += 1;
      return registration;
    },
  } as unknown as ServiceWorkerContainer;

  assert.equal(await ensureServiceWorkerRegistration(container), registration);
  assert.equal(registerCalls, 0);
});

void test("service worker registration creates the root registration once when missing", async () => {
  const registration = {
    scope: "https://wiki.example/",
  } as ServiceWorkerRegistration;
  let registerCalls = 0;
  const container = {
    getRegistration: async () => undefined,
    register: async (
      scriptURL: string | URL,
      options?: RegistrationOptions,
    ) => {
      registerCalls += 1;
      assert.equal(scriptURL, "/sw.js");
      assert.equal(options?.scope, "/");
      return registration;
    },
  } as unknown as ServiceWorkerContainer;

  assert.equal(await ensureServiceWorkerRegistration(container), registration);
  assert.equal(registerCalls, 1);
});

void test("prefixed registration uses the exact deployment scope and rejects an ancestor registration", async () => {
  const oldDocument = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: { body: { dataset: { routePrefix: "/kumbuka" } } },
  });
  try {
    const registration = {
      scope: "https://wiki.example/kumbuka/",
    } as ServiceWorkerRegistration;
    let found = { scope: "https://wiki.example/" } as
      ServiceWorkerRegistration | undefined;
    let calls = 0;
    const container = {
      getRegistration: async (scope: string) => {
        assert.equal(scope, "/kumbuka/");
        return found;
      },
      register: async (script: string, options: RegistrationOptions) => {
        calls++;
        assert.equal(script, "/kumbuka/sw.js");
        assert.equal(options.scope, "/kumbuka/");
        return registration;
      },
    } as unknown as ServiceWorkerContainer;
    assert.equal(
      await ensureServiceWorkerRegistration(container),
      registration,
    );
    found = registration;
    assert.equal(
      await ensureServiceWorkerRegistration(container),
      registration,
    );
    assert.equal(calls, 1);
    found = undefined;
    assert.equal(
      await ensureServiceWorkerRegistration(container),
      registration,
    );
    assert.equal(calls, 2);
  } finally {
    if (oldDocument) Object.defineProperty(globalThis, "document", oldDocument);
    else Reflect.deleteProperty(globalThis, "document");
  }
});

void test("manifest start, scope, and icons resolve within the deployment", () => {
  const manifest = JSON.parse(
    readFileSync("web/src/manifest.webmanifest", "utf8"),
  ) as {
    start_url: string;
    scope: string;
    icons: { src: string }[];
  };
  for (const prefix of ["", "/kumbuka"]) {
    const url = `https://wiki.example${prefix}/assets/v-test/manifest.webmanifest`;
    assert.equal(new URL(manifest.start_url, url).pathname, `${prefix}/`);
    assert.equal(new URL(manifest.scope, url).pathname, `${prefix}/`);
    assert.equal(
      new URL(manifest.icons[0]!.src, url).pathname,
      `${prefix}/assets/v-test/favicon.svg`,
    );
  }
});
