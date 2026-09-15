import test from "node:test";
import assert from "node:assert/strict";

import { formatMarkdownDocument } from "../../web/src/ts/features/editor/formatter.ts";

test("formats a complete Markdown document conservatively", () => {
  const source = [
    "",
    "##   Heading ",
    "",
    "",
    "+   first item ",
    "* second item",
    "",
    ">quoted text ",
    "",
    "| Name|State |",
    "|---|:---:|",
    "| API| healthy|",
    "| Database |warning |",
    "",
    "",
  ].join("\n");

  assert.equal(
    formatMarkdownDocument(source),
    [
      "## Heading",
      "",
      "- first item",
      "- second item",
      "",
      "> quoted text",
      "",
      "| Name     | State   |",
      "| -------- | :-----: |",
      "| API      | healthy |",
      "| Database | warning |",
      "",
    ].join("\n"),
  );
});

test("preserves fenced and indented block contents", () => {
  const source = [
    "# Title",
    "",
    "```bash   ",
    "printf '  keep  spaces  '   ",
    "* not a list",
    "```",
    "",
    '=== "Linux"',
    "",
    "    * keep this tab body exactly   ",
    "    ```bash",
    "    echo 'inside'   ",
    "    ```",
  ].join("\n");

  assert.equal(
    formatMarkdownDocument(source),
    [
      "# Title",
      "",
      "```bash",
      "printf '  keep  spaces  '   ",
      "* not a list",
      "```",
      "",
      '=== "Linux"',
      "",
      "    * keep this tab body exactly   ",
      "    ```bash",
      "    echo 'inside'   ",
      "    ```",
      "",
    ].join("\n"),
  );
});

test("preserves Markdown hard line breaks", () => {
  assert.equal(
    formatMarkdownDocument("first line    \nsecond line\n"),
    "first line  \nsecond line\n",
  );
});

test("formatting is idempotent", () => {
  const source = "|A|B|\n|---|---:|\n| one |2|\n\n\n-  item\n";
  const once = formatMarkdownDocument(source);

  assert.equal(formatMarkdownDocument(once), once);
});

test("preserves image widths in ordinary, linked and reference images", () => {
  const source = [
    "# Images",
    "",
    '![Diagram](/media/42/diagram.png "Overview"){width=640px}',
    "",
    "[![Diagram](/media/42/diagram.png){width=50%}](/media/42/diagram.png)",
    "",
    "![Diagram][image]{width=50%}",
    "",
    "[image]: /media/42/diagram.png",
    "",
    "`![Example](example.png){width=50%}`",
    "",
  ].join("\n");

  assert.equal(formatMarkdownDocument(source), source);
  assert.equal(formatMarkdownDocument(formatMarkdownDocument(source)), source);
});
