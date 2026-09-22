import test from "node:test";
import assert from "node:assert/strict";
import {
  canonicalCodeLanguage,
  codeLanguageLabel,
  filterCodeLanguages,
  supportedCodeLanguages,
} from "../../web/src/ts/features/editor/code-languages.ts";

test("code language catalog includes bundled highlighter languages", () => {
  const values = supportedCodeLanguages().map((language) => language.value);

  assert.ok(values.includes("go"));
  assert.ok(values.includes("python"));
  assert.ok(values.includes("javascript"));
  assert.ok(values.includes("yaml"));
  assert.equal(values[0], "");
});

test("code language labels normalize common fence aliases", () => {
  assert.equal(canonicalCodeLanguage(" js "), "javascript");
  assert.equal(codeLanguageLabel("js"), "JavaScript");
  assert.equal(codeLanguageLabel("yml"), "YAML");
  assert.equal(codeLanguageLabel(""), "Plain text");
});

test("code language filtering searches labels identifiers and aliases", () => {
  assert.deepEqual(
    filterCodeLanguages("python").map((language) => language.value),
    ["python", "python_2"],
  );
  assert.ok(
    filterCodeLanguages("js").some(
      (language) => language.value === "javascript",
    ),
  );
});
