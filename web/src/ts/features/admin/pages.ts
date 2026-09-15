// Administrator bulk page operations.

import { requestConfirmation } from "../../core/dialogs.ts";
import { requiredElement } from "../../core/dom.ts";

// Wires bulk page selection and destructive-action confirmation.
export function initAdminPages(): void {
  const bulk = document.querySelector<HTMLFormElement>("[data-bulk-pages]");
  if (!bulk) return;

  const pages = [
    ...bulk.querySelectorAll<HTMLInputElement>("[data-bulk-page]"),
  ];
  const selectAll = requiredElement<HTMLInputElement>(
    bulk,
    "[data-bulk-select-all]",
  );
  const count = requiredElement<HTMLElement>(bulk, "[data-bulk-count]");
  const action = requiredElement<HTMLSelectElement>(bulk, "[data-bulk-action]");
  const submit = requiredElement<HTMLButtonElement>(bulk, "[data-bulk-submit]");
  const fields = [...bulk.querySelectorAll<HTMLElement>("[data-bulk-field]")];

  // Refreshes the active administration page state.
  const refresh = (): void => {
    const selected = pages.filter((input) => input.checked).length;

    count.textContent = `${selected} selected`;
    selectAll.checked = selected > 0 && selected === pages.length;
    selectAll.indeterminate = selected > 0 && selected < pages.length;
    submit.disabled = !selected || !action.value;

    fields.forEach((field) => {
      field.hidden = field.dataset.bulkField !== action.value;
    });
  };

  pages.forEach((input) => input.addEventListener("change", refresh));
  selectAll.addEventListener("change", () => {
    pages.forEach((input) => {
      input.checked = selectAll.checked;
    });
    refresh();
  });
  action.addEventListener("change", refresh);
  bulk.addEventListener("submit", async (event: SubmitEvent) => {
    if (bulk.dataset.confirming === "true" || action.value !== "delete") {
      return;
    }

    event.preventDefault();

    const selected = pages.filter((input) => input.checked).length;
    const accepted = await requestConfirmation(
      `Move ${selected} selected page${selected === 1 ? "" : "s"} to the recycle bin?`,
      {
        eyebrow: "Bulk pages",
        title: "Delete selected pages?",
        confirmLabel: "Move to bin",
        cancelLabel: "Cancel",
        danger: true,
      },
    );
    if (!accepted) return;

    bulk.dataset.confirming = "true";
    bulk.requestSubmit();
  });
  refresh();
}
