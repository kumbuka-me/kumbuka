import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { localPasswordProblem } from "../../web/src/ts/core/password.ts";

const cases = JSON.parse(
  readFileSync("test/contracts/passwords.json", "utf8"),
) as {
  name: string;
  password: string;
  problem: string;
}[];

for (const entry of cases) {
  test(`local password matches Go: ${entry.name}`, () => {
    assert.equal(localPasswordProblem(entry.password), entry.problem);
  });
}
