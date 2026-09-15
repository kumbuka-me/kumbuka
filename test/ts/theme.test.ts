import assert from "node:assert/strict";
import test from "node:test";

import { parseThemeCatalog } from "../../web/src/ts/core/theme.ts";

void test("parseThemeCatalog accepts complete server theme definitions", () => {
  assert.deepEqual(
    parseThemeCatalog(
      JSON.stringify([
        {
          title: "Light",
          color_scheme: "light",
          colors: { background: "#ffffff", text: "#111111" },
        },
      ]),
    ),
    [
      {
        title: "Light",
        color_scheme: "light",
        colors: { background: "#ffffff", text: "#111111" },
      },
    ],
  );
});

void test("parseThemeCatalog rejects malformed color values", () => {
  let caught: unknown;

  try {
    parseThemeCatalog(
      JSON.stringify([
        {
          title: "Broken",
          color_scheme: "dark",
          colors: { background: 7 },
        },
      ]),
    );
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Invalid theme catalog.");
});

void test("parseThemeCatalog rejects unknown color schemes", () => {
  let caught: unknown;

  try {
    parseThemeCatalog(
      JSON.stringify([
        {
          title: "Broken",
          color_scheme: "auto",
          colors: { background: "#000000" },
        },
      ]),
    );
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Invalid theme catalog.");
});
