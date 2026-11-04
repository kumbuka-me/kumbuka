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

test("disabled plugin languages are absent unless explicitly contributed", () => {
  const runbook = { value: "runbook", label: "Runbook" };
  const mermaid = { value: "mermaid", label: "Mermaid" };
  assert.ok(
    !supportedCodeLanguages().some((language) => language.value === "runbook"),
  );
  assert.ok(
    !supportedCodeLanguages().some((language) => language.value === "mermaid"),
  );
  assert.deepEqual(filterCodeLanguages("runbook", [runbook]), [runbook]);
  assert.deepEqual(filterCodeLanguages("mermaid", [mermaid]), [mermaid]);
  assert.equal(codeLanguageLabel("runbook", [runbook]), "Runbook");
});
