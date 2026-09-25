import assert from "node:assert/strict";
import test from "node:test";

import {
  matchWidgetMacro,
  matchWidgetSource,
  normalizeWidgetColor,
  parseMacro,
  rewriteWidgetMacro,
  rewriteWidgetSource,
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

test("mention and date settings validate canonical persisted values", () => {
  const widget: CatalogWidget = {
    plugin_id: "me.example.assignment",
    id: "assignment",
    name: "Assignment",
    inline: false,
    syntax: { kind: "macro", name: "assignment" },
    attributes: [
      { name: "assignee", type: "string", max_bytes: 128 },
      { name: "due", type: "string", max_bytes: 10 },
    ],
    settings: [
      { type: "mention", label: "Assignee", attribute: "assignee" },
      { type: "date", label: "Due", attribute: "due" },
    ],
    preview: {
      kind: "card",
      card: { class: "assignment", title: "Assignment" },
    },
  };

  assert.deepEqual(
    validateWidgetValues({ assignee: "@alice", due: "2026-09-25" }, widget),
    [],
  );
  const problems = validateWidgetValues(
    { assignee: "alice", due: "2026-02-30" },
    widget,
  );
  assert.ok(problems.some((problem) => problem.includes("@mention")));
  assert.ok(problems.some((problem) => problem.includes("YYYY-MM-DD")));
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

test("widget macro parser accepts bare values and repeated attributes", () => {
  const widget: CatalogWidget = {
    plugin_id: "me.kumbuka.external-files",
    id: "external-file",
    name: "External file",
    inline: false,
    syntax: { kind: "macro", name: "external-file" },
    attributes: [
      { name: "source", type: "string", required: true },
      { name: "path", type: "string", required: true },
      {
        name: "note",
        type: "list",
        separator: "\u001f",
        repeat: true,
      },
      { name: "line-numbers", type: "enum", values: ["true", "false"] },
    ],
    settings: [
      { type: "text", label: "Source", attribute: "source" },
      { type: "text", label: "Path", attribute: "path" },
      {
        type: "table",
        label: "Notes",
        attributes: ["note"],
        columns: [{ label: "Note", type: "textarea" }],
      },
      { type: "select", label: "Line numbers", attribute: "line-numbers" },
    ],
    preview: {
      kind: "card",
      card: { class: "external-file", title: "External file" },
    },
  };
  const raw =
    '{{external-file source="docs" path="README.md" note="3:First" note="8:Second" line-numbers=true}}';
  const values = widgetValues(raw, widget);

  assert.equal(values.note, "3:First\u001f8:Second");
  values.note = "3:First\u001f9:Changed";
  assert.equal(
    rewriteWidgetMacro(raw, widget, values),
    '{{external-file source="docs" path="README.md" note="3:First" note="9:Changed" line-numbers="true"}}',
  );
});

test("substitution widgets round-trip their resource reference", () => {
  const widget: CatalogWidget = {
    plugin_id: "me.kumbuka.includes",
    id: "include",
    name: "Include",
    inline: true,
    syntax: { kind: "substitution", name: "include" },
    attributes: [{ name: "target", type: "string", required: true }],
    settings: [{ type: "text", label: "Target", attribute: "target" }],
    preview: {
      kind: "reference",
      reference: {
        class: "visual-include-reference",
        prefix: "Include",
        value_attribute: "target",
      },
    },
  };
  const raw = "{{include:operations/postgres#Restore from backup}}";
  const matched = matchWidgetSource(raw + " after", [widget], true);

  assert.equal(matched?.raw, raw);
  assert.equal(
    widgetValues(raw, widget).target,
    "operations/postgres#Restore from backup",
  );
  assert.equal(
    rewriteWidgetSource(raw, widget, { target: "operations/shared-warning" }),
    "{{include:operations/shared-warning}}",
  );
});

test("callout and details widgets preserve structured block content", () => {
  const callout: CatalogWidget = {
    plugin_id: "me.kumbuka.callouts",
    id: "callout",
    name: "Callout",
    inline: false,
    syntax: { kind: "callout" },
    attributes: [
      {
        name: "kind",
        type: "enum",
        required: true,
        values: ["note", "warning"],
      },
      { name: "body", type: "string", required: true },
    ],
    settings: [
      { type: "select", label: "Type", attribute: "kind" },
      { type: "textarea", label: "Content", attribute: "body" },
    ],
    preview: {
      kind: "callout",
      callout: {
        class: "callout",
        kind_attribute: "kind",
        body_attribute: "body",
      },
    },
  };
  const details: CatalogWidget = {
    plugin_id: "me.kumbuka.details",
    id: "details",
    name: "Details",
    inline: false,
    syntax: { kind: "details" },
    attributes: [
      { name: "title", type: "string", required: true },
      { name: "open", type: "enum", values: ["false", "true"] },
      { name: "body", type: "string", required: true },
    ],
    settings: [
      { type: "text", label: "Title", attribute: "title" },
      { type: "select", label: "Open", attribute: "open" },
      { type: "textarea", label: "Body", attribute: "body" },
    ],
    preview: {
      kind: "details",
      details: {
        class: "markdown-details",
        title_attribute: "title",
        open_attribute: "open",
        body_attribute: "body",
      },
    },
  };

  const calloutRaw = "!!! warning\nBack up the database.";
  assert.deepEqual(widgetValues(calloutRaw, callout), {
    kind: "warning",
    body: "Back up the database.",
  });
  assert.equal(
    rewriteWidgetSource(calloutRaw, callout, {
      kind: "note",
      body: "Updated body.",
    }),
    "!!! note\nUpdated body.",
  );

  const detailsRaw = '???+ "Show command"\n\n    **Markdown**';
  assert.deepEqual(widgetValues(detailsRaw, details), {
    title: "Show command",
    open: "true",
    body: "**Markdown**",
  });
  assert.equal(
    rewriteWidgetSource(detailsRaw, details, {
      title: "Show command",
      open: "false",
      body: "**Updated**",
    }),
    '??? "Show command"\n\n    **Updated**',
  );
});

test("tabs widgets map parallel titles and bodies without losing multiline content", () => {
  const widget: CatalogWidget = {
    plugin_id: "me.kumbuka.tabs",
    id: "tabs",
    name: "Tabs",
    inline: false,
    syntax: { kind: "tabs" },
    attributes: [
      { name: "titles", type: "list", required: true, separator: "\u001f" },
      { name: "bodies", type: "list", required: true, separator: "\u001f" },
    ],
    settings: [
      {
        type: "table",
        label: "Tabs",
        attributes: ["titles", "bodies"],
        columns: [
          { label: "Title", type: "text" },
          { label: "Content", type: "textarea" },
        ],
      },
    ],
    constraints: [{ kind: "same-length", attributes: ["titles", "bodies"] }],
    preview: {
      kind: "tabs",
      tabs: {
        class: "markdown-tabs",
        list_class: "markdown-tab-list",
        tab_class: "markdown-tab",
        panels_class: "markdown-tab-panels",
        panel_class: "markdown-tab-panel",
        titles_attribute: "titles",
        bodies_attribute: "bodies",
      },
    },
  };
  const raw = '=== "Linux"\n\n    **apt**\n\n=== "macOS"\n\n    `brew`';
  const values = widgetValues(raw, widget);

  assert.equal(values.titles, "Linux\u001fmacOS");
  assert.equal(values.bodies, "**apt**\u001f`brew`");
  assert.equal(rewriteWidgetSource(raw, widget, values), raw);
  assert.deepEqual(validateWidgetValues(values, widget), []);
});

test("optional defaults can preserve an explicit empty macro attribute", () => {
  const widget: CatalogWidget = {
    plugin_id: "me.kumbuka.subpages",
    id: "subpages",
    name: "Subpages",
    inline: false,
    syntax: { kind: "macro", name: "subpages" },
    attributes: [
      {
        name: "title",
        type: "string",
        default: "Pages in this section",
        emit_empty: true,
      },
    ],
    settings: [{ type: "text", label: "Heading", attribute: "title" }],
    preview: {
      kind: "card",
      card: { class: "subpage-toc", title: "Subpages" },
    },
  };

  assert.equal(
    rewriteWidgetMacro("{{subpages}}", widget, { title: "" }),
    '{{subpages title=""}}',
  );
});
