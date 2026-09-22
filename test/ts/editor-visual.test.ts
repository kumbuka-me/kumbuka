import assert from "node:assert/strict";
import test from "node:test";

import {
  matchKumbukaBlockSyntax,
  matchKumbukaInlineSyntax,
  visualSyntaxLabel,
} from "../../web/src/ts/features/editor/visual-syntax.ts";

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
