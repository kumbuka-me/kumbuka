import assert from "node:assert/strict";
import test from "node:test";

import {
  initPerformance,
  measure,
  performanceStatus,
  setPerformanceEnabled,
} from "../../web/src/ts/core/performance.ts";

interface PerformanceAPI {
  enable(): boolean;
  disable(): boolean;
  status(): { available: boolean; enabled: boolean; serverTiming: boolean };
}

class MemoryStorage implements Storage {
  private readonly values = new Map<string, string>();

  get length(): number {
    return this.values.size;
  }

  clear(): void {
    this.values.clear();
  }

  getItem(key: string): string | null {
    return this.values.get(key) ?? null;
  }

  key(index: number): string | null {
    return [...this.values.keys()][index] ?? null;
  }

  removeItem(key: string): void {
    this.values.delete(key);
  }

  setItem(key: string, value: string): void {
    this.values.set(key, value);
  }
}

test("deployment-gated performance diagnostics expose browser controls", () => {
  const descriptors = new Map(
    ["window", "localStorage", "document", "location", "kumbuka"].map(
      (name) =>
        [name, Object.getOwnPropertyDescriptor(globalThis, name)] as const,
    ),
  );
  let cookie = "";

  Object.defineProperty(globalThis, "window", {
    configurable: true,
    value: globalThis,
  });
  Object.defineProperty(globalThis, "localStorage", {
    configurable: true,
    value: new MemoryStorage(),
  });
  Object.defineProperty(globalThis, "document", {
    configurable: true,
    value: {
      body: {
        dataset: {
          routePrefix: "/kumbuka",
          performanceDiagnostics: "true",
        },
      },
      get cookie() {
        return cookie;
      },
      set cookie(value: string) {
        cookie = value;
      },
    },
  });
  Object.defineProperty(globalThis, "location", {
    configurable: true,
    value: {
      href: "https://example.test/kumbuka/pages/example",
      origin: "https://example.test",
      protocol: "https:",
    },
  });

  try {
    initPerformance();
    assert.deepEqual(performanceStatus(), {
      available: true,
      enabled: false,
      serverTiming: false,
    });

    const api = (
      globalThis as typeof globalThis & {
        kumbuka?: { perf?: PerformanceAPI };
      }
    ).kumbuka?.perf;
    assert.ok(api);

    assert.equal(setPerformanceEnabled(true), true);
    assert.equal(api.status().enabled, true);
    assert.ok(/kumbuka_perf=1/.test(cookie));
    assert.ok(/Path=\/kumbuka\//.test(cookie));

    const value = measure("unit-test", () => 42);
    assert.equal(value, 42);
    assert.ok(
      performance
        .getEntriesByType("measure")
        .some((entry) => entry.name === "kumbuka:unit-test"),
    );

    assert.equal(api.disable(), true);
    assert.equal(api.status().enabled, false);
    assert.ok(/Max-Age=0/.test(cookie));
  } finally {
    performance.clearMarks();
    performance.clearMeasures();
    for (const [name, descriptor] of descriptors) {
      if (descriptor) Object.defineProperty(globalThis, name, descriptor);
      else Reflect.deleteProperty(globalThis, name);
    }
  }
});
