import assert from "node:assert/strict";
import test from "node:test";

import {
  matchKumbukaBlockSyntax,
  matchKumbukaInlineSyntax,
  visualSyntaxLabel,
} from "../../web/src/ts/features/editor/visual-syntax.ts";
import { renumberMarkdownHeadings } from "../../web/src/ts/features/editor/heading-numbering.ts";

test("visual editor preserves Kumbuka macros as atomic inline syntax", () => {
  assert.equal(
    matchKumbukaInlineSyntax('{{status id="ready" set="release"}} after'),
    '{{status id="ready" set="release"}}',
  );
  assert.equal(visualSyntaxLabel('{{status id="ready"}}'), "Status");
});

test("visual editor preserves wiki links as atomic inline syntax", () => {
  assert.equal(
    matchKumbukaInlineSyntax("[[platform/api|API docs]] after"),
    "[[platform/api|API docs]]",
  );
  assert.equal(visualSyntaxLabel("[[platform/api|API docs]]"), "API docs");
});

test("visual editor preserves standalone directives", () => {
  const directive = "{table header=accent sortable}";
  assert.equal(matchKumbukaBlockSyntax(directive + "\nNext"), directive);
  assert.equal(visualSyntaxLabel(directive), "Table options");
});

test("visual editor does not consume ordinary Markdown braces", () => {
  assert.equal(matchKumbukaInlineSyntax("{ordinary text}"), null);
  assert.equal(matchKumbukaBlockSyntax("some {ordinary text}"), null);
});

test("automatic heading numbering preserves Markdown source and fenced code", () => {
  assert.equal(
    renumberMarkdownHeadings(
      "# Introduction\n\n## Install\n\n```md\n# Literal\n```\n",
    ),
    "# 1. Introduction\n\n## 1.1 Install\n\n```md\n# Literal\n```\n",
  );
});

test("heading numbering preserves code, line endings, and years in titles", () => {
  const source =
    "    # indented\r\n~~~\r\n~~~ text\r\n# fenced\r\n~~~~\r\n# 2026 report\r\n";
  const want = source.replace("# 2026 report", "# 1. 2026 report");
  assert.equal(renumberMarkdownHeadings(source), want);
  assert.equal(renumberMarkdownHeadings(want), want);
});
