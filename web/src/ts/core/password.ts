import { t } from "./i18n.ts";

// Local password policy shared by all new-password form controls.
// Keep these boundaries aligned with internal/auth/password.go and the shared fixtures.

const minimumCharacters = 12;
const maximumBytes = 72;

export function localPasswordProblem(password: string): string {
  if ([...password].length < minimumCharacters)
    return t("browser.validation.password_min", "Use at least 12 characters.");
  if (new TextEncoder().encode(password).length > maximumBytes)
    return t("browser.validation.password_max", "Use at most 72 UTF-8 bytes.");
  return "";
}
