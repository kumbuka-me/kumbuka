import assert from "node:assert/strict";
import test from "node:test";
import { exportParameterOverrides } from "../../web/src/ts/features/exports.ts";
import { parseExportPreview } from "../../web/src/ts/features/export-preview.ts";

test("export parameters omit unchanged values and preserve explicit empty values", () => {
  assert.deepEqual(
    exportParameterOverrides([
      {
        pluginID: "me.kumbuka.variables",
        moduleID: "values",
        key: "environment",
        saved: "production",
        value: "production",
      },
    ]),
    {},
  );
  assert.deepEqual(
    exportParameterOverrides([
      {
        pluginID: "me.kumbuka.variables",
        moduleID: "values",
        key: "environment",
        saved: "production",
        value: "",
      },
    ]),
    { "me.kumbuka.variables": { values: { environment: "" } } },
  );
});

test("export parameters preserve whitespace and prototype-like keys", () => {
  const values = exportParameterOverrides([
    {
      pluginID: "me.kumbuka.variables",
      moduleID: "values",
      key: "__proto__",
      saved: "old",
      value: " staging\n ",
    },
  ]);
  assert.equal(
    JSON.stringify(values),
    '{"me.kumbuka.variables":{"values":{"__proto__":" staging\\n "}}}',
  );
});

test("export preview accepts a complete HTML document", () => {
  assert.equal(
    parseExportPreview({ document: "<!doctype html><p>staging</p>" }),
    "<!doctype html><p>staging</p>",
  );
});
