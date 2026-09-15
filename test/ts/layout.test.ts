import test from "node:test";
import assert from "node:assert/strict";

import { sidebarWidthStatus } from "../../web/src/ts/features/layout.ts";

test("sidebarWidthStatus labels presets and custom widths", () => {
  assert.equal(sidebarWidthStatus(240), "Narrow · 240 px");
  assert.equal(sidebarWidthStatus(280), "Standard · 280 px");
  assert.equal(sidebarWidthStatus(360), "Wide · 360 px");
  assert.equal(sidebarWidthStatus(297), "Custom · 297 px");
});

test("sidebarWidthStatus clamps values to supported bounds", () => {
  assert.equal(sidebarWidthStatus(100), "Custom · 220 px");
  assert.equal(sidebarWidthStatus(999), "Custom · 420 px");
});
