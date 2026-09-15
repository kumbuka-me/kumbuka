import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";

void test("selected palette is applied synchronously before the body exists", () => {
  const properties = new Map<string, string>();
  const root = {
    dataset: { theme: "custom dark" },
    style: {
      colorScheme: "",
      setProperty: (key: string, value: string) => properties.set(key, value),
    },
  };
  runInNewContext(readFileSync("web/dist/js/theme-init.js", "utf8"), {
    document: {
      documentElement: root,
      body: null,
      getElementById: () => ({
        textContent: JSON.stringify([
          { title: "Light", color_scheme: "light", colors: {} },
          {
            title: "Custom Dark",
            color_scheme: "dark",
            colors: { background: "#101010", surface_elevated: "#202020" },
          },
        ]),
      }),
    },
  });
  assert.equal(root.dataset.theme, "Custom Dark");
  assert.equal(root.style.colorScheme, "dark");
  assert.equal(properties.get("--background"), "#101010");
  assert.equal(properties.get("--surface-elevated"), "#202020");
});

void test("themed layouts load the blocking initializer immediately after the catalog", () => {
  for (const path of [
    "web/src/templates/layout.gohtml",
    "web/src/templates/shared_page.gohtml",
    "internal/site/templates/layout.gohtml",
  ]) {
    assert.ok(
      /<script id="kumbuka-themes"[^>]*>.*?<\/script>\s*<script src="[^\n]*theme-init\.js[^\n]*"><\/script>/.test(
        readFileSync(path, "utf8"),
      ),
    );
  }
});
