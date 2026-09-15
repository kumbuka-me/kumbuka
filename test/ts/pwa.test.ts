import assert from "node:assert/strict";
import test from "node:test";

import { ensureServiceWorkerRegistration } from "../../web/src/ts/core/pwa.ts";

void test("service worker registration reuses an existing root registration", async () => {
  const registration = {} as ServiceWorkerRegistration;
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
  const registration = {} as ServiceWorkerRegistration;
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
