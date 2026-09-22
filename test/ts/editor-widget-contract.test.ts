import assert from "node:assert/strict";
import test from "node:test";

import {
  matchWidgetMacro,
  normalizeWidgetColor,
  parseMacro,
  rewriteWidgetMacro,
  splitWidgetList,
  validateWidgetValues,
  widgetValues,
  type CatalogWidget,
} from "../../web/src/ts/features/editor/widget-contract.ts";

const statusWidget: CatalogWidget = {
  plugin_id: "me.kumbuka.status-dropdowns",
  id: "status",
  name: "Status",
  inline: true,
  syntax: { kind: "macro", name: "status" },
  attributes: [
    { name: "id", type: "identifier", required: true, max_bytes: 128 },
    { name: "set", type: "identifier", max_bytes: 128 },
    {
      name: "options",
      type: "list",
      max_bytes: 64,
      max_items: 16,
      separator: ";",
      fallback_separator: ",",
      unique: true,
    },
    {
      name: "colors",
      type: "color-list",
      max_bytes: 16,
      max_items: 16,
      separator: ";",
      fallback_separator: ",",
      aliases: { blue: "#2563eb", red: "#dc2626" },
    },
    { name: "initial", type: "string", max_bytes: 64 },
    { name: "prefix", type: "string", max_bytes: 64 },
    {
      name: "style",
      type: "enum",
      default: "solid",
      values: ["solid", "outline"],
    },
  ],
  settings: [
    { type: "text", label: "ID", attribute: "id" },
    {
      type: "text",
      label: "Reusable set",
      attribute: "set",
      suggestions: ["workflow", "approval"],
    },
    {
      type: "table",
      label: "Page-local statuses",
      attributes: ["options", "colors"],
      columns: [
        { label: "Status", type: "text" },
        { label: "Color", type: "color" },
      ],
    },
    { type: "text", label: "Initial status", attribute: "initial" },
    { type: "text", label: "Prefix", attribute: "prefix" },
    { type: "select", label: "Style", attribute: "style" },
  ],
  constraints: [
    { kind: "exactly-one", attributes: ["set", "options"] },
    {
      kind: "same-length",
      attributes: ["options", "colors"],
      optional: true,
    },
    {
      kind: "member-of",
      attributes: ["initial", "options"],
      optional: true,
    },
  ],
  preview: {
    kind: "badge",
    badge: {
      class: "kumbuka-status",
      labels_attribute: "options",
      colors_attribute: "colors",
      style_attribute: "style",
      default_label: "Status",
      default_colors: ["#64748b", "#2563eb"],
    },
  },
};

test("widget macro parser preserves declared and unknown attributes", () => {
  const raw =
    '{{status id="release" options="To do;Done" future="keep me" prefix="Release"}}';
  const macro = parseMacro(raw);

  assert.ok(macro);
  assert.equal(macro.name, "status");
  assert.deepEqual(macro.attributes, [
    { name: "id", value: "release" },
    { name: "options", value: "To do;Done" },
    { name: "future", value: "keep me" },
    { name: "prefix", value: "Release" },
  ]);
  assert.deepEqual(matchWidgetMacro(raw + " after", [statusWidget], true), {
    raw,
    widget: statusWidget,
  });
});

test("widget macro rewrite retains unknown attributes and omitted defaults", () => {
  const raw =
    '{{status id="release" options="To do;Done" future="keep me" prefix="Release"}}';
  const values = widgetValues(raw, statusWidget);
  values.prefix = "Deploy";

  assert.equal(
    rewriteWidgetMacro(raw, statusWidget, values),
    '{{status id="release" options="To do;Done" future="keep me" prefix="Deploy"}}',
  );
});

test("status list mapping accepts legacy comma syntax and emits named colors safely", () => {
  const options = statusWidget.attributes.find(
    (item) => item.name === "options",
  );
  const colors = statusWidget.attributes.find((item) => item.name === "colors");
  assert.ok(options);
  assert.ok(colors);

  assert.deepEqual(splitWidgetList("To do,In progress,Done", options), [
    "To do",
    "In progress",
    "Done",
  ]);
  assert.deepEqual(splitWidgetList("Ready, steady;Go", options), [
    "Ready, steady",
    "Go",
  ]);
  assert.equal(normalizeWidgetColor("blue", colors), "#2563eb");
});

test("status widget validates page-local rows and cross-field rules", () => {
  assert.deepEqual(
    validateWidgetValues(
      {
        id: "release",
        set: "",
        options: "To do;Done",
        colors: "blue;red",
        initial: "Done",
        prefix: "Release",
        style: "outline",
      },
      statusWidget,
    ),
    [],
  );

  const problems = validateWidgetValues(
    {
      id: "release",
      set: "workflow",
      options: "Done;Done",
      colors: "blue",
      initial: "Missing",
      prefix: "",
      style: "solid",
    },
    statusWidget,
  );
  assert.ok(problems.some((problem) => problem.includes("exactly one")));
  assert.ok(problems.some((problem) => problem.includes("duplicate")));
  assert.ok(problems.some((problem) => problem.includes("same number")));
  assert.ok(problems.some((problem) => problem.includes("initial")));
});

test("multiline macro matching remains opt-in per plugin contract", () => {
  const multiline = {
    ...statusWidget,
    inline: false,
    syntax: { kind: "macro" as const, name: "status", multiline: true },
  };
  const source = '{{status\n id="release"\n set="workflow"\n}}\nNext';

  assert.equal(matchWidgetMacro(source, [statusWidget], true), null);
  assert.equal(
    matchWidgetMacro(source, [multiline], false)?.raw.includes("\n"),
    true,
  );
});

test("macro attributes require separating whitespace", () => {
  assert.equal(parseMacro('{{status id="a"set="workflow"}}'), null);
});

test("widget values do not inherit undeclared object properties", () => {
  const values = widgetValues('{{status id="a" set="workflow"}}', statusWidget);
  assert.equal(values.constructor, undefined);
  assert.equal(values.toString, undefined);
});

test("block widgets do not consume the beginning of ordinary paragraphs", () => {
  const widget = { ...statusWidget, inline: false };
  assert.equal(
    matchWidgetMacro(
      '{{status id="a" set="workflow"}} trailing',
      [widget],
      false,
    ),
    null,
  );
});
