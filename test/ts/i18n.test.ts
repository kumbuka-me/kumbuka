import test from "node:test";
import assert from "node:assert/strict";

import { locale, t } from "../../web/src/ts/core/i18n.ts";

test("browser i18n falls back cleanly without a document", () => {
  assert.equal(locale(), "en");
  assert.equal(
    t("missing", "Hello {name}", { name: "Kumbuka" }),
    "Hello Kumbuka",
  );
});
