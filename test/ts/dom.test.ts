import assert from "node:assert/strict";
import test from "node:test";

import {
  requiredAttribute,
  requiredElement,
  requiredElements,
} from "../../web/src/ts/core/dom.ts";

void test("requiredElement returns the matching template element", () => {
  const expected = {} as Element;
  const root = {
    querySelector: () => expected,
  } as unknown as ParentNode;

  assert.equal(requiredElement(root, "[data-required]"), expected);
});

void test("requiredElement fails loudly when required markup is missing", () => {
  const root = {
    querySelector: () => null,
  } as unknown as ParentNode;
  let caught: unknown;

  try {
    requiredElement(root, "[data-required]");
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Missing required element: [data-required]");
});

void test("requiredElements returns all matching template elements", () => {
  const expected = [{}, {}] as Element[];
  const root = {
    querySelectorAll: () => expected,
  } as unknown as ParentNode;

  assert.deepEqual(requiredElements(root, "[data-required]"), expected);
});

void test("requiredElements fails loudly when required markup is missing", () => {
  const root = {
    querySelectorAll: () => [],
  } as unknown as ParentNode;
  let caught: unknown;

  try {
    requiredElements(root, "[data-required]");
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Missing required elements: [data-required]");
});

void test("requiredAttribute returns a trimmed required attribute", () => {
  const element = {
    getAttribute: () => "  /api/pages  ",
  } as unknown as Element;

  assert.equal(requiredAttribute(element, "data-url"), "/api/pages");
});

void test("requiredAttribute fails loudly when an attribute is absent", () => {
  const element = {
    getAttribute: () => null,
  } as unknown as Element;
  let caught: unknown;

  try {
    requiredAttribute(element, "data-url");
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Missing required attribute: data-url");
});
