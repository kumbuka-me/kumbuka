// Reusable confirmation and notice dialog helpers.

import { requiredElement, requiredElements } from "./dom.ts";
import type { ProblemPayload } from "./http.ts";

interface ConfirmationOptions {
  eyebrow?: string;
  title?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  danger?: boolean;
}

interface NoticeOptions {
  title?: string;
}

export interface ProblemDialogDetail {
  field: string;
  label: string;
  message: string;
}

interface ProblemDialogOptions {
  title?: string;
  details?: ProblemDialogDetail[];
}

function confirmationDialog(): HTMLDialogElement | null {
  if (typeof document === "undefined") return null;
  return document.querySelector<HTMLDialogElement>("[data-confirm-dialog]");
}

function problemDialog(): HTMLDialogElement | null {
  if (typeof document === "undefined") return null;
  return document.querySelector<HTMLDialogElement>("[data-problem-dialog]");
}

// Returns the shared notice dialog elements.
function noticeDialog(): HTMLDialogElement | null {
  if (typeof document === "undefined") return null;
  return document.querySelector<HTMLDialogElement>("[data-notice-dialog]");
}

// Wires the application About dialog to its footer trigger.
export function initAboutDialog(): void {
  if (typeof document === "undefined") return;

  const dialog = document.querySelector<HTMLDialogElement>("[data-about-dialog]");
  if (!dialog) return;

  const close = (): void => {
    if (dialog.open) dialog.close();
  };

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-about-open]",
  )) {
    button.addEventListener("click", () => {
      if (!dialog.open) dialog.showModal();
    });
  }

  for (const button of dialog.querySelectorAll<HTMLButtonElement>(
    "[data-about-close]",
  )) {
    button.addEventListener("click", close);
  }

  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) close();
  });
}

// Shows a confirmation dialog and resolves with the user choice.
export function requestConfirmation(
  message: string,
  options: ConfirmationOptions = {},
): Promise<boolean> {
  const dialog = confirmationDialog();
  if (!dialog) return Promise.resolve(false);

  const eyebrow = requiredElement<HTMLElement>(
    dialog,
    "[data-confirm-dialog-eyebrow]",
  );
  const title = requiredElement<HTMLElement>(
    dialog,
    "[data-confirm-dialog-title]",
  );
  const body = requiredElement<HTMLElement>(
    dialog,
    "[data-confirm-dialog-message]",
  );
  const accept = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-confirm-dialog-accept]",
  );
  const cancelLabel = requiredElement<HTMLElement>(
    dialog,
    "[data-confirm-dialog-cancel-label]",
  );
  const cancelButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-confirm-dialog-cancel]",
  );

  eyebrow.textContent = options.eyebrow || "Confirmation";
  title.textContent = options.title || "Confirm action";
  body.textContent = message || "Continue?";
  accept.textContent = options.confirmLabel || "Continue";
  cancelLabel.textContent = options.cancelLabel || "Cancel";

  accept.classList.toggle("danger", options.danger !== false);
  accept.classList.toggle("primary", options.danger === false);

  return new Promise<boolean>((resolve) => {
    let settled = false;
    // Completes the active dialog request.
    const finish = (value: boolean) => {
      if (settled) return;

      settled = true;
      dialog.close();
      resolve(value);
    };

    for (const button of cancelButtons) button.onclick = () => finish(false);

    accept.onclick = () => finish(true);
    dialog.oncancel = (event: Event) => {
      event.preventDefault();
      finish(false);
    };
    dialog.onclick = (event: MouseEvent) => {
      if (event.target === dialog) finish(false);
    };
    dialog.showModal();
  });
}

// Converts a request field name into readable fallback copy for the error dialog.
export function problemFieldLabel(field: string): string {
  const normalized = field.trim().replaceAll("_", " ").replaceAll("-", " ");
  if (!normalized) return "Field";

  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
}

// Shows a server problem without exposing the raw JSON response to browser users.
export function showProblemDialog(
  problem: ProblemPayload,
  options: ProblemDialogOptions = {},
): Promise<boolean> {
  const dialog = problemDialog();
  if (!dialog) return Promise.resolve(false);

  const title = requiredElement<HTMLElement>(
    dialog,
    "[data-problem-dialog-title]",
  );
  const body = requiredElement<HTMLElement>(
    dialog,
    "[data-problem-dialog-message]",
  );
  const details = requiredElement<HTMLElement>(
    dialog,
    "[data-problem-dialog-details]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-problem-dialog-close]",
  );

  title.textContent = options.title || "Could not complete this action";
  body.textContent =
    problem.error?.trim() || "The request could not be processed.";
  details.replaceChildren();

  const entries =
    options.details ??
    Object.entries(problem.problems ?? {}).map(([field, message]) => ({
      field,
      label: problemFieldLabel(field),
      message,
    }));

  for (const detail of entries) {
    if (!detail.message.trim()) continue;

    const term = document.createElement("dt");
    const description = document.createElement("dd");
    term.textContent = detail.label || problemFieldLabel(detail.field);
    description.textContent = detail.message;
    details.append(term, description);
  }

  details.hidden = details.childElementCount === 0;

  return new Promise<boolean>((resolve) => {
    let settled = false;
    const finish = () => {
      if (settled) return;

      settled = true;
      dialog.close();
      resolve(true);
    };

    for (const button of closeButtons) button.onclick = finish;

    dialog.oncancel = (event: Event) => {
      event.preventDefault();
      finish();
    };
    dialog.onclick = (event: MouseEvent) => {
      if (event.target === dialog) finish();
    };
    dialog.showModal();
  });
}

// Shows a simple notice dialog.
export function showNotice(
  message: string,
  options: NoticeOptions = {},
): Promise<void> {
  const dialog = noticeDialog();
  if (!dialog) return Promise.resolve();

  const title = requiredElement<HTMLElement>(
    dialog,
    "[data-notice-dialog-title]",
  );
  const body = requiredElement<HTMLElement>(
    dialog,
    "[data-notice-dialog-message]",
  );
  const closeButtons = requiredElements<HTMLButtonElement>(
    dialog,
    "[data-notice-dialog-close]",
  );

  title.textContent = options.title || "Notice";
  body.textContent = message || "";

  return new Promise<void>((resolve) => {
    let settled = false;
    // Completes the active dialog request.
    const finish = () => {
      if (settled) return;

      settled = true;
      dialog.close();
      resolve();
    };

    for (const button of closeButtons) button.onclick = finish;

    dialog.oncancel = (event: Event) => {
      event.preventDefault();
      finish();
    };
    dialog.onclick = (event: MouseEvent) => {
      if (event.target === dialog) finish();
    };
    dialog.showModal();
  });
}

// Returns the form or submit control that owns confirmation metadata for one submission.
function confirmationSource(
  form: HTMLFormElement,
  submitter: HTMLElement | null,
): HTMLElement | null {
  if (submitter?.hasAttribute("data-confirm")) return submitter;
  if (form.hasAttribute("data-confirm")) return form;

  return null;
}

// Reports whether a form can request confirmation for at least one submission path.
function hasConfirmation(form: HTMLFormElement): boolean {
  return (
    form.hasAttribute("data-confirm") ||
    form.querySelector(
      'button[data-confirm], input[type="submit"][data-confirm], input[type="image"][data-confirm]',
    ) !== null
  );
}

// Initializes confirmation handling for forms and submitter-specific destructive actions.
export function initConfirmForms(): void {
  if (typeof document === "undefined") return;

  for (const form of document.querySelectorAll<HTMLFormElement>("form")) {
    if (!hasConfirmation(form)) continue;

    form.addEventListener("submit", async (event: SubmitEvent) => {
      if (form.dataset.confirmBypass === "true") {
        delete form.dataset.confirmBypass;
        return;
      }

      const submitter =
        event.submitter instanceof HTMLElement ? event.submitter : null;
      const source = confirmationSource(form, submitter);
      if (!source) return;

      event.preventDefault();

      const accepted = await requestConfirmation(
        source.dataset.confirm || "Continue?",
        {
          title: source.dataset.confirmTitle || "Confirm action",
          confirmLabel: source.dataset.confirmLabel || "Continue",
          cancelLabel: source.dataset.confirmCancelLabel || "Cancel",
          eyebrow: source.dataset.confirmEyebrow || "Confirmation",
          danger: source.dataset.confirmDanger !== "false",
        },
      );
      if (!accepted) return;

      form.dataset.confirmBypass = "true";

      if (submitter) form.requestSubmit(submitter);
      else form.requestSubmit();
    });
  }
}
