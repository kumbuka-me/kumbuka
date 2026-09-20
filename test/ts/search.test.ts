import assert from "node:assert/strict";
import test from "node:test";
import { initSearch } from "../../web/src/ts/features/search.ts";

// Exercise dismissal against the real search event handlers, including a response
// that completes despite cancellation (as can happen after fetch has resolved).
void test("live search cancels queued and in-flight work when dismissed", async () => {
  class Element extends EventTarget {
    hidden = false;
    className = "";
    value = "matomo";
    attributes = new Map<string, string>();
    children: Element[] = [];
    setAttribute(name: string, value: string): void {
      this.attributes.set(name, value);
    }
    append(child: Element): void {
      this.children.push(child);
    }
    replaceChildren(): void {
      this.children = [];
    }
    contains(child: unknown): boolean {
      return child === this || this.children.includes(child as Element);
    }
    blur(): void {}
  }
  const input = new Element();
  const form = Object.assign(new Element(), { querySelector: () => input });
  form.append(input);
  const documentStub = Object.assign(new Element(), {
    querySelectorAll: () => [form],
    createElement: () => new Element(),
  });
  const originals = {
    document: globalThis.document,
    Node: globalThis.Node,
    fetch: globalThis.fetch,
  };
  Object.assign(globalThis, { document: documentStub, Node: Element });
  let calls = 0;
  let finish: ((response: Response) => void) | undefined;
  let signal: AbortSignal | null | undefined;
  globalThis.fetch = async (_url, options) => {
    calls++;
    signal = options?.signal;
    return new Promise<Response>((resolve) => {
      finish = resolve;
    });
  };
  const wait = (ms: number) => new Promise<void>((resolve) => setTimeout(resolve, ms));
  const escape = () =>
    input.dispatchEvent(Object.assign(new Event("keydown"), { key: "Escape" }));
  try {
    initSearch();
    const results = form.children[1]!;
    input.dispatchEvent(new Event("input"));
    escape();
    await wait(150);
    assert.equal(calls, 0, "Escape cancels the debounce timer");
    assert.equal(results.hidden, true);

    input.dispatchEvent(new Event("focus"));
    await wait(10);
    assert.equal(calls, 1);
    escape();
    assert.equal(signal?.aborted, true);
    finish!(new Response("[]", { headers: { "Content-Type": "application/json" } }));
    await wait(0);
    assert.equal(results.hidden, true, "A late response cannot reopen dismissed results");

    input.dispatchEvent(new Event("input"));
    form.dispatchEvent(Object.assign(new Event("focusout"), { relatedTarget: new Element() }));
    await wait(150);
    assert.equal(calls, 1, "Leaving the search with Tab cancels queued work");
  } finally {
    Object.assign(globalThis, originals);
  }
});
