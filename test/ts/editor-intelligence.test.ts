import test from "node:test";
import assert from "node:assert/strict";

import {
  editorDiagnostics,
  editorSlug,
  markdownHeadings,
  wikiLinkRanges,
} from "../../web/src/ts/features/editor/intelligence.ts";

test("editorSlug follows Kumbuka page path rules", () => {
  assert.equal(editorSlug(" PostgreSQL Restore "), "postgresql-restore");
  assert.equal(
    editorSlug("Applications / PostgreSQL"),
    "applications-/-postgresql",
  );
});

test("markdownHeadings ignores fenced code and preserves offsets", () => {
  const source = "# One\n\n```md\n## Hidden\n```\n\n### Three\n";
  const headings = markdownHeadings(source);

  assert.deepEqual(
    headings.map(({ level, title }) => ({ level, title })),
    [
      { level: 1, title: "One" },
      { level: 3, title: "Three" },
    ],
  );
  assert.equal(source.slice(headings[1].offset).startsWith("### Three"), true);
});

test("wikiLinkRanges extracts targets and labels", () => {
  assert.deepEqual(wikiLinkRanges("See [[Postgres|database]] and [[Redis]]."), [
    { start: 4, end: 25, target: "Postgres", raw: "[[Postgres|database]]" },
    { start: 30, end: 39, target: "Redis", raw: "[[Redis]]" },
  ]);
});

test("editorDiagnostics reports broken links, heading jumps, and link suggestions", () => {
  const catalog = {
    pages: [
      { slug: "postgresql", title: "PostgreSQL" },
      { slug: "redis", title: "Redis" },
    ],
    aliases: { "old-postgres": "postgresql" },
    completions: [
      {
        plugin_id: "me.kumbuka.variables",
        module_id: "variables",
        trigger: "{{",
        label: "cluster",
        replacement: "{{var:cluster}}",
      },
      {
        plugin_id: "me.kumbuka.snippets",
        module_id: "snippets",
        trigger: "{{",
        label: "warning",
        replacement: "{{snippet:warning}}",
      },
    ],
    completion_providers: [],
    inserts: [],
  };
  const source = [
    "# Runbook",
    "### Procedure",
    "See [[missing]] and [[old-postgres]].",
    "PostgreSQL is documented elsewhere.",
  ].join("\n");
  const diagnostics = editorDiagnostics(source, catalog, "runbook");

  assert.equal(
    diagnostics.some(
      (item) => item.code === "broken-link" && item.title.includes("missing"),
    ),
    true,
  );
  assert.equal(
    diagnostics.some((item) => item.code === "heading-jump"),
    true,
  );
  assert.equal(
    diagnostics.some(
      (item) =>
        item.code === "link-suggestion" &&
        item.replacement === "[[PostgreSQL]]",
    ),
    true,
  );
  assert.equal(
    diagnostics.some((item) => item.title.includes("old-postgres")),
    false,
  );
});
