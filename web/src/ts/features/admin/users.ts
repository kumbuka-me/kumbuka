// Administrator user access and external identity editors.

import {
  requiredAttribute,
  requiredElement,
  requiredElements,
} from "../../core/dom.ts";

// Returns group identifiers encoded on a user row action.
function selectedGroupIDs(button: HTMLElement): Set<string> {
  return new Set(
    (button.dataset.groupIds || "").trim().split(/\s+/).filter(Boolean),
  );
}

// Wires role and group editing for registered users.
export function setupAdminUserEditor(dialog: HTMLDialogElement): void {
  const editorForm = requiredElement<HTMLFormElement>(
    dialog,
    "[data-admin-user-form]",
  );
  const editorName = requiredElement<HTMLElement>(
    dialog,
    "[data-admin-user-name]",
  );
  const editorIdentity = requiredElement<HTMLElement>(
    dialog,
    "[data-admin-user-identity]",
  );
  const editorRole = requiredElement<HTMLSelectElement>(
    dialog,
    "[data-admin-user-role]",
  );
  const accountEnabled = requiredElement<HTMLInputElement>(
    dialog,
    "[data-admin-user-enabled]",
  );
  const localLogin = requiredElement<HTMLElement>(
    dialog,
    "[data-admin-user-local-login]",
  );
  const localCredentialEnabled = requiredElement<HTMLInputElement>(
    dialog,
    "[data-admin-user-local-credential-enabled]",
  );
  const localCredentialUpdate = requiredElement<HTMLInputElement>(
    dialog,
    "[data-admin-user-local-credential-update]",
  );
  const localPassword = requiredElement<HTMLDetailsElement>(
    dialog,
    "[data-admin-user-local-password]",
  );
  const localPasswordInput = requiredElement<HTMLInputElement>(
    dialog,
    "[data-admin-user-local-password-input]",
  );
  const localPasswordConfirm = requiredElement<HTMLInputElement>(
    dialog,
    "[data-admin-user-local-password-confirm]",
  );
  const groupInputs = [
    ...dialog.querySelectorAll<HTMLInputElement>('input[name="group_id"]'),
  ];

  localPassword.addEventListener("toggle", () => {
    for (const input of [localPasswordInput, localPasswordConfirm]) {
      input.disabled = !localPassword.open;

      if (!localPassword.open) input.value = "";
    }
  });

  function openUserEditor(button: HTMLButtonElement): void {
    const userID = requiredAttribute(button, "data-user-id");
    const groups = selectedGroupIDs(button);

    editorForm.reset();
    editorForm.action = `/admin/users/${encodeURIComponent(userID)}`;
    editorName.textContent =
      button.dataset.userName || button.dataset.userUsername || "Edit user";

    const username = button.dataset.userUsername || "";
    const email = button.dataset.userEmail || "";

    editorIdentity.textContent = email ? `${username} · ${email}` : username;
    editorRole.value = button.dataset.userRole || "viewer";

    accountEnabled.checked = button.dataset.userEnabled === "true";

    const hasLocalCredential = button.dataset.userHasLocalCredential === "true";
    const canManageLocalCredential =
      hasLocalCredential && dialog.dataset.externalAuthActive === "true";

    localLogin.hidden = !canManageLocalCredential;
    localCredentialEnabled.checked =
      button.dataset.userLocalCredentialEnabled === "true";
    localCredentialEnabled.disabled = !canManageLocalCredential;
    localCredentialUpdate.disabled = !canManageLocalCredential;

    localPassword.open = false;
    localPasswordInput.disabled = true;
    localPasswordConfirm.disabled = true;
    localPasswordInput.value = "";
    localPasswordConfirm.value = "";

    for (const input of groupInputs) input.checked = groups.has(input.value);

    dialog.showModal();
    requestAnimationFrame(() => editorRole.focus());
  }

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-admin-user-edit]",
  )) {
    button.addEventListener("click", () => openUserEditor(button));
  }

  for (const button of requiredElements<HTMLButtonElement>(
    dialog,
    "[data-admin-user-close]",
  )) {
    button.addEventListener("click", () => dialog.close());
  }

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}

// Wires administrator decisions for unknown verified OIDC identities.
export function setupPendingOIDCEditor(dialog: HTMLDialogElement): void {
  const identityName = requiredElement<HTMLElement>(
    dialog,
    "[data-pending-oidc-name]",
  );
  const identityProfile = requiredElement<HTMLElement>(
    dialog,
    "[data-pending-oidc-profile]",
  );
  const identityIssuer = requiredElement<HTMLElement>(
    dialog,
    "[data-pending-oidc-issuer]",
  );
  const identitySubject = requiredElement<HTMLElement>(
    dialog,
    "[data-pending-oidc-subject]",
  );
  const identityUser = requiredElement<HTMLSelectElement>(
    dialog,
    "[data-pending-oidc-user]",
  );
  const match = dialog.querySelector<HTMLElement>("[data-pending-oidc-match]");
  const identityLinkForm = requiredElement<HTMLFormElement>(
    dialog,
    "[data-pending-oidc-link-form]",
  );
  const identityApproveForm = requiredElement<HTMLFormElement>(
    dialog,
    "[data-pending-oidc-approve-form]",
  );
  const identityRejectForm = requiredElement<HTMLFormElement>(
    dialog,
    "[data-pending-oidc-reject-form]",
  );

  function openPendingIdentityEditor(button: HTMLButtonElement): void {
    const pendingID = requiredAttribute(button, "data-pending-id");

    identityName.textContent =
      button.dataset.pendingName || "Review OIDC identity";

    const username = button.dataset.pendingUsername || "";
    const email = button.dataset.pendingEmail || "";

    identityProfile.textContent = email ? `${username} · ${email}` : username;
    identityIssuer.textContent = button.dataset.pendingIssuer || "";
    identitySubject.textContent = button.dataset.pendingSubject || "";

    const base = `/admin/oidc/pending/${encodeURIComponent(pendingID)}`;

    identityLinkForm.action = `${base}/link`;
    identityApproveForm.action = `${base}/approve`;
    identityRejectForm.action = `${base}/reject`;

    const suggestedID = button.dataset.suggestedUserId || "";

    identityUser.value = [...identityUser.options].some(
      (option) => option.value === suggestedID,
    )
      ? suggestedID
      : "";

    if (match) {
      const suggestedName = button.dataset.suggestedUserName || "";

      match.hidden = !suggestedID;
      match.textContent = suggestedName
        ? `Possible match: ${suggestedName}`
        : "Possible matching Kumbuka account found.";
    }

    dialog.showModal();
    requestAnimationFrame(() => identityUser.focus());
  }

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-pending-oidc-review]",
  )) {
    button.addEventListener("click", () => openPendingIdentityEditor(button));
  }

  for (const button of requiredElements<HTMLButtonElement>(
    dialog,
    "[data-pending-oidc-close]",
  )) {
    button.addEventListener("click", () => dialog.close());
  }

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}
