import test from "node:test";
import assert from "node:assert/strict";
import {
  matchingSlashCommands,
  slashCommandTrigger,
} from "../../web/src/ts/features/editor/commands.ts";

test("slashCommandTrigger activates only at the current line start", () => {
  assert.deepEqual(slashCommandTrigger("# Page\n/ta", 10), {
    start: 7,
    query: "ta",
  });
  assert.equal(slashCommandTrigger("text /ta", 8), null);
});

test("slash commands filter by label and description", () => {
  assert.equal(matchingSlashCommands("table")[0].id, "table");
  assert.ok(
    matchingSlashCommands("collapsible").some((item) => item.id === "details"),
  );
  assert.equal(matchingSlashCommands("mention")[0].id, "mention");
});
