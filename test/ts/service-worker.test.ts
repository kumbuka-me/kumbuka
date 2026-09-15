import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

function worker() {
  const listeners = new Map<string, (event: any) => void>();
  const stored = new Map<string, Map<string, Response>>();
  const runtime = {
    self: {
      location: { origin: "https://wiki.example" },
      addEventListener: (type: string, listener: (event: any) => void) =>
        listeners.set(type, listener),
    },
    URL,
    Response,
    fetch: async (_request: Request): Promise<Response> => {
      throw new Error("offline");
    },
    caches: {
      keys: async () => [...stored.keys()],
      delete: async (name: string) => stored.delete(name),
      open: async (name: string) => {
        if (!stored.has(name)) stored.set(name, new Map());
        const cache = stored.get(name)!;
        return {
          put: async (request: Request, response: Response) => {
            cache.set(request.url, response);
          },
          match: async (request: Request) => cache.get(request.url),
        };
      },
    },
  };
  runInNewContext(readFileSync("web/dist/sw.js", "utf8"), runtime);

  const message = async (data: unknown, clientID = "tab-1"): Promise<void> => {
    let finished: Promise<void> = Promise.resolve();

    listeners.get("message")!({
      data,
      source: { id: clientID },
      waitUntil: (value: Promise<void>) => {
        finished = value;
      },
    });
    await finished;
  };

  return {
    runtime,
    stored,
    message,
    configure: (userID: string, clientID = "tab-1") =>
      message({ type: "configure-user", userID }, clientID),
    request: (clientID = "tab-1", path = "/pages/start") => {
      let response: Promise<Response> | undefined;
      listeners.get("fetch")!({
        request: new Request("https://wiki.example" + path),
        clientId: clientID,
        respondWith: (value: Promise<Response>) => {
          response = value;
        },
      });
      return response;
    },
  };
}

test("private page caching requires a configured client and matching server identity", async () => {
  const w = worker();
  await w.configure("7");
  assert.equal(w.request("unconfigured-tab"), undefined);
  w.runtime.fetch = async () =>
    new Response("private", { headers: { "X-Kumbuka-User-ID": "7" } });
  await w.request();
  assert.equal(w.stored.get("kumbuka-pages-v2-7")?.size, 1);
  w.runtime.fetch = async () => {
    throw new Error("offline");
  };
  assert.equal(await (await w.request())?.text(), "private");
});

test("in-flight responses cannot cross an account switch or logout", async () => {
  for (const nextUser of ["8", ""]) {
    const w = worker();
    await w.configure("7");
    let resolve!: (response: Response) => void;
    w.runtime.fetch = () =>
      new Promise((done) => {
        resolve = done;
      });
    const pending = w.request();
    await w.configure(nextUser);
    resolve(
      new Response("old private page", {
        headers: { "X-Kumbuka-User-ID": "7" },
      }),
    );
    await pending;
    assert.equal(
      [...w.stored.values()].reduce((sum, cache) => sum + cache.size, 0),
      0,
    );
  }
});

test("login responses and mismatched identities are never cached", async () => {
  const w = worker();
  await w.configure("7");
  for (const user of ["", "8"]) {
    w.runtime.fetch = async () =>
      new Response("other user", { headers: { "X-Kumbuka-User-ID": user } });
    await w.request();
  }
  assert.equal(w.stored.size, 0);
});

test("versioned assets are served from the service-worker cache", async () => {
  const w = worker();
  const path = "/assets/v-release-one/app.js";
  w.runtime.fetch = async () => new Response("release-one");
  assert.equal(await (await w.request("tab-1", path))?.text(), "release-one");

  w.runtime.fetch = async () => {
    throw new Error("cached asset should not hit the network");
  };
  assert.equal(await (await w.request("tab-1", path))?.text(), "release-one");
});

test("malformed configure-user messages are ignored instead of coerced", async () => {
  const w = worker();

  await w.configure("7");
  await w.message({ type: "configure-user", userID: 8 });

  w.runtime.fetch = async () =>
    new Response("private", { headers: { "X-Kumbuka-User-ID": "7" } });

  const response = w.request();
  assert.ok(response);
  await response;
  assert.equal(w.stored.get("kumbuka-pages-v2-7")?.size, 1);
});
