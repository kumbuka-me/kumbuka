// Administrator feature bootstrap.

import { initAdminConfiguration } from "./configuration.ts";
import { setupGroupMemberPicker } from "./groups.ts";
import { initAdminImport } from "./import.ts";
import { setupNavigationIconPicker } from "./navigation.ts";
import { initAdminPages } from "./pages.ts";
import { initAdminPlugins } from "./plugins.ts";
import { setupAdminUserEditor, setupPendingOIDCEditor } from "./users.ts";
import { initAdminWebhooks } from "./webhooks.ts";

// Initializes administrator features present on the current page.
export function initAdmin(): void {
  const navigationIconPicker = document.querySelector<HTMLDialogElement>(
    "[data-icon-picker-dialog]",
  );
  if (navigationIconPicker) setupNavigationIconPicker(navigationIconPicker);

  for (const card of document.querySelectorAll<HTMLElement>(
    "[data-group-members]",
  )) {
    setupGroupMemberPicker(card);
  }

  const userEditor = document.querySelector<HTMLDialogElement>(
    "[data-admin-user-dialog]",
  );
  if (userEditor) setupAdminUserEditor(userEditor);

  const identityEditor = document.querySelector<HTMLDialogElement>(
    "[data-pending-oidc-dialog]",
  );
  if (identityEditor) setupPendingOIDCEditor(identityEditor);

  initAdminConfiguration();
  initAdminImport();
  initAdminPages();
  initAdminPlugins();
  initAdminWebhooks();
}
