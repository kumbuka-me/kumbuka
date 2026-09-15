// Personal access token creation and revocation UI.

import { setupCopyButton } from "../core/clipboard.ts";
import { requestConfirmation, showNotice } from "../core/dialogs.ts";
import { requiredAttribute } from "../core/dom.ts";
import { isRecord } from "../core/guards.ts";
import { errorMessage, requestJSON } from "../core/http.ts";

interface TokenRecord {
  id: number;
  name: string;
  username: string;
  creator: string;
  expires_at?: string | null;
}

interface CreateTokenResponse {
  secret: string;
  token: TokenRecord;
}

function isTokenRecord(value: unknown): value is TokenRecord {
  return (
    isRecord(value) &&
    typeof value.id === "number" &&
    typeof value.name === "string" &&
    typeof value.username === "string" &&
    typeof value.creator === "string" &&
    (value.expires_at === undefined ||
      value.expires_at === null ||
      typeof value.expires_at === "string")
  );
}

function isCreateTokenResponse(value: unknown): value is CreateTokenResponse {
  return (
    isRecord(value) &&
    typeof value.secret === "string" &&
    isTokenRecord(value.token)
  );
}

// Renders token secret.
function renderTokenSecret(container: HTMLElement, secret: string): void {
  container.hidden = false;
  container.replaceChildren();

  const message = document.createElement("p");

  message.textContent = "This token is shown only once.";

  const block = document.createElement("div");

  block.className = "code-block token-secret-value";

  const pre = document.createElement("pre");
  const code = document.createElement("code");

  code.textContent = secret;
  pre.append(code);

  const copy = document.createElement("button");

  copy.type = "button";
  copy.className = "code-copy-button";
  setupCopyButton(copy, () => secret, "Copy token to clipboard");
  block.append(pre, copy);
  container.append(message, block);
}

// Prepends token row.
function prependTokenRow(form: HTMLFormElement, token: TokenRecord): void {
  const list =
    form.parentElement?.querySelector<HTMLElement>("[data-token-list]");
  if (!list) return;

  list.querySelector("[data-token-empty]")?.remove();

  const row = document.createElement("tr");

  row.dataset.tokenRow = "";

  const expires = token.expires_at ? token.expires_at.slice(0, 10) : "Never";
  const admin = form.action.includes("/admin/tokens");

  row.innerHTML = admin
    ? `<td><strong></strong><small></small></td><td></td><td>just now</td>` +
      `<td><span class="muted">Never</span></td><td></td><td></td>`
    : `<td><strong></strong></td><td>just now</td>` +
      `<td><span class="muted">Never</span></td><td></td><td></td>`;

  const strong = row.querySelector<HTMLElement>("strong");

  if (strong) strong.textContent = token.name;

  const cells = row.querySelectorAll<HTMLTableCellElement>("td");

  if (admin) {
    const small = row.querySelector<HTMLElement>("small");

    if (small) small.textContent = `issued by ${token.creator}`;
    if (cells[1]) cells[1].textContent = token.username;
    if (cells[4]) cells[4].textContent = expires;
  } else if (cells[3]) {
    cells[3].textContent = expires;
  }

  const actionCell = cells[cells.length - 1];
  if (!actionCell) return;

  const revoke = document.createElement("button");

  revoke.type = "button";
  revoke.className = "button danger";
  revoke.dataset.tokenDelete = "";
  revoke.dataset.deleteUrl = admin
    ? `/admin/tokens/${token.id}`
    : `/settings/tokens/${token.id}`;
  revoke.textContent = "Revoke";
  setupTokenDeleteButton(revoke);
  actionCell.append(revoke);
  list.prepend(row);
}

function formBody(form: HTMLFormElement): URLSearchParams {
  const params = new URLSearchParams();

  for (const [key, value] of new FormData(form).entries()) {
    params.append(key, String(value));
  }

  return params;
}

async function createToken(form: HTMLFormElement): Promise<void> {
  const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]');
  const secret = form.parentElement?.querySelector<HTMLElement>(
    "[data-token-secret]",
  );
  if (!secret || !submit) return;

  submit.disabled = true;

  try {
    const payload = await requestJSON(form.action, {
      method: "POST",
      headers: {
        "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
      },
      body: formBody(form),
    });
    if (!isCreateTokenResponse(payload))
      throw new Error("Invalid token response.");

    renderTokenSecret(secret, payload.secret);
    prependTokenRow(form, payload.token);
    form.reset();
  } catch (error) {
    console.error("token creation failed", error);
    await showNotice(errorMessage(error) || "Token could not be created.", {
      title: "Token creation failed",
    });
  } finally {
    submit.disabled = false;
  }
}

// Wires token forms behavior.
function setupTokenForms(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-token-form]",
  )) {
    form.addEventListener("submit", (event: SubmitEvent) => {
      event.preventDefault();
      void createToken(form);
    });
  }
}

// Revokes one token after confirmation.
async function revokeToken(button: HTMLButtonElement): Promise<void> {
  if (
    !(await requestConfirmation(
      "Revoke this token? Requests using it will stop working immediately.",
      { title: "Revoke access token", confirmLabel: "Revoke token" },
    ))
  )
    return;

  const deleteURL = requiredAttribute(button, "data-delete-url");

  button.disabled = true;

  try {
    await requestJSON(deleteURL, { method: "DELETE" });

    button.closest("[data-token-row]")?.remove();
  } catch (error) {
    console.error("token revocation failed", error);
    await showNotice(errorMessage(error) || "Token could not be revoked.", {
      title: "Token revocation failed",
    });
    button.disabled = false;
  }
}

// Wires token delete button behavior.
function setupTokenDeleteButton(button: HTMLButtonElement): void {
  button.addEventListener("click", () => void revokeToken(button));
}

// Initializes tokens.
export function initTokens(): void {
  setupTokenForms();
  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-token-delete]",
  ))
    setupTokenDeleteButton(button);
}
