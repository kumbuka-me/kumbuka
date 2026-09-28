import assert from "node:assert/strict";
import test from "node:test";

import { profileFieldPresentation } from "../../web/src/ts/features/admin/users.ts";

test("local profile fields stay directly editable", () => {
  assert.deepEqual(profileFieldPresentation("local", "username", false), {
    state: "Local",
    providerVisible: false,
    revertVisible: false,
    editable: true,
  });
});

test("OIDC fields expose provider ownership and per-field revert", () => {
  assert.equal(
    profileFieldPresentation("oidc", "email", false).state,
    "Provider-managed",
  );
  assert.equal(
    profileFieldPresentation("oidc", "email", false).revertVisible,
    false,
  );
  assert.equal(
    profileFieldPresentation("oidc", "email", true).state,
    "Locally overridden",
  );
  assert.equal(
    profileFieldPresentation("oidc", "email", true).revertVisible,
    true,
  );
});

test("trusted-proxy username requires relinking while mutable fields can be overridden", () => {
  const username = profileFieldPresentation("trusted-proxy", "username", true);
  assert.equal(username.editable, false);
  assert.equal(username.revertVisible, false);

  const displayName = profileFieldPresentation(
    "trusted-proxy",
    "displayName",
    true,
  );
  assert.equal(displayName.editable, true);
  assert.equal(displayName.revertVisible, true);
});
