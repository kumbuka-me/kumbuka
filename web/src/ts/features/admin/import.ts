// Import form pending-state handling.

import { requiredElement } from "../../core/dom.ts";

// selectedImportFileCount returns the number of files currently selected across both import inputs.
function selectedImportFileCount(form: HTMLFormElement): number {
  let count = 0;

  for (const input of form.querySelectorAll<HTMLInputElement>(
    'input[type="file"][name="files"]',
  )) {
    count += input.files?.length ?? 0;
  }

  return count;
}

// setupImportForm shows immediate feedback while the blocking import request is running.
function setupImportForm(form: HTMLFormElement): void {
  const submit = requiredElement<HTMLButtonElement>(
    form,
    "[data-import-submit]",
  );
  const icon = requiredElement<HTMLElement>(form, "[data-import-submit-icon]");
  const spinner = requiredElement<HTMLElement>(
    form,
    "[data-import-submit-spinner]",
  );
  const label = requiredElement<HTMLElement>(
    form,
    "[data-import-submit-label]",
  );
  const progress = requiredElement<HTMLElement>(form, "[data-import-progress]");

  let submitting = false;

  form.addEventListener("submit", (event: SubmitEvent) => {
    if (submitting) return;

    event.preventDefault();
    submitting = true;

    const fileCount = selectedImportFileCount(form);
    progress.textContent =
      fileCount > 1
        ? `Importing ${fileCount} selected files…`
        : "Importing pages…";
    progress.hidden = false;

    form.setAttribute("aria-busy", "true");
    submit.disabled = true;
    icon.hidden = true;
    spinner.hidden = false;
    label.textContent = "Importing…";

    // Give the browser one paint before navigation starts so the pending state is visible.
    requestAnimationFrame(() => HTMLFormElement.prototype.submit.call(form));
  });
}

// initAdminImport initializes import forms present on administration pages.
export function initAdminImport(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-import-form]",
  )) {
    setupImportForm(form);
  }
}
