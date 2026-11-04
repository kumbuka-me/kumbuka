// Audit inspection dialogs expose recorded event details and exact page revisions.

import { requiredAttribute, requiredElement } from "../../core/dom.ts";
import { loadRevisionHistory } from "../revision-history.ts";

// initAdminAudit opens audit events in a dialog without navigating to an HTML fragment.
export function initAdminAudit(): void {
  const dialog = document.querySelector<HTMLDialogElement>(
    "[data-audit-detail-dialog]",
  );
  if (!dialog) return;

  const title = requiredElement<HTMLElement>(
    dialog,
    "[data-audit-dialog-title]",
  );
  const actor = requiredElement<HTMLElement>(
    dialog,
    "[data-audit-dialog-actor]",
  );
  const target = requiredElement<HTMLElement>(
    dialog,
    "[data-audit-dialog-target]",
  );
  const time = requiredElement<HTMLElement>(dialog, "[data-audit-dialog-time]");
  const summary = requiredElement<HTMLElement>(
    dialog,
    "[data-audit-dialog-summary]",
  );
  const history = requiredElement<HTMLElement>(
    dialog,
    "[data-audit-dialog-history]",
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-audit-dialog-close]",
  );
  let activeRequest: AbortController | null = null;

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-audit-detail-open]",
  )) {
    button.addEventListener("click", () => {
      activeRequest?.abort();
      activeRequest = new AbortController();
      const action = requiredAttribute(button, "data-audit-action");
      const url = button.dataset.auditRevisionUrl;
      const number = Number(button.dataset.auditRevisionNumber || 0);

      title.textContent = action;
      actor.textContent = button.dataset.auditActor || "System";
      target.textContent = button.dataset.auditTarget || "Not specified";
      time.textContent = button.dataset.auditTime || "Unknown";
      summary.textContent =
        button.dataset.auditDetail || "No additional details recorded.";
      history.replaceChildren();
      history.hidden = !url;
      dialog.showModal();
      if (url)
        void loadRevisionHistory(history, url, number, activeRequest.signal);
    });
  }

  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", () => activeRequest?.abort());
  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
}
