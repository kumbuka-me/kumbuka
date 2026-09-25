// Shared behavior for plugin-provided visual-editor forms.

import { requestConfirmation } from "../../core/dialogs.ts";

const enhancedAttribute = "data-visual-widget-popover-enhanced";
const closeBypassAttribute = "data-visual-widget-close-bypass";
const textareaEnhancedAttribute = "data-visual-widget-textarea-enhanced";

type WidgetControl = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

// formSignature returns the editable state used to detect unapplied form changes.
function formSignature(form: HTMLFormElement): string {
  return JSON.stringify(
    Array.from(
      form.querySelectorAll<WidgetControl>("input, select, textarea"),
    ).map((control) => ({
      tag: control.tagName,
      type: control instanceof HTMLInputElement ? control.type : "",
      attribute: control.dataset.widgetAttribute || "",
      tableAttribute: control.dataset.widgetTableAttribute || "",
      value: control.value,
    })),
  );
}

// resizeLongformTextarea grows a table textarea with its content up to a bounded height.
function resizeLongformTextarea(textarea: HTMLTextAreaElement): void {
  textarea.style.height = "0px";
  const height = Math.min(160, Math.max(44, textarea.scrollHeight));
  textarea.style.height = `${height}px`;
  textarea.style.overflowY = textarea.scrollHeight > 160 ? "auto" : "hidden";
}

// enhanceLongformTables applies the compact note-style table treatment.
function enhanceLongformTables(popover: HTMLElement): void {
  for (const table of popover.querySelectorAll<HTMLTableElement>(
    ".visual-widget-table",
  )) {
    const textareas = table.querySelectorAll<HTMLTextAreaElement>("textarea");
    if (textareas.length === 0) continue;

    table.classList.add("visual-widget-table-longform");
    table
      .closest<HTMLElement>(".visual-widget-table-field")
      ?.classList.add("visual-widget-table-field-longform");

    const headers = Array.from(table.tHead?.rows[0]?.cells || []).map(
      (cell) => cell.textContent?.trim() || "",
    );
    if (headers[0] === "Line(s)" && headers[1] === "Note")
      table.classList.add("visual-widget-table-line-notes");

    for (const textarea of textareas) {
      if (!textarea.hasAttribute(textareaEnhancedAttribute)) {
        textarea.setAttribute(textareaEnhancedAttribute, "");
        textarea.rows = 1;
        textarea.addEventListener("input", () =>
          resizeLongformTextarea(textarea),
        );
      }
      resizeLongformTextarea(textarea);
    }
  }
}

// enhancePopover adds sticky chrome and guarded cancellation to one plugin form.
function enhancePopover(popover: HTMLElement): void {
  enhanceLongformTables(popover);

  if (popover.hasAttribute(enhancedAttribute)) return;
  popover.setAttribute(enhancedAttribute, "");

  const heading = popover.querySelector<HTMLElement>(
    ":scope > .visual-widget-popover-title",
  );
  const form = popover.querySelector<HTMLFormElement>(":scope > form");
  const cancel = form?.querySelector<HTMLButtonElement>(
    ".visual-widget-popover-actions button:nth-last-child(2)",
  );
  if (!heading || !form || !cancel) return;

  const header = document.createElement("div");
  header.className = "visual-widget-popover-header";

  const close = document.createElement("button");
  close.type = "button";
  close.className = "icon-button visual-widget-popover-close";
  close.setAttribute("aria-label", `Close ${heading.textContent || "plugin form"}`);
  close.title = "Close";
  close.textContent = "×";

  heading.before(header);
  header.append(heading, close);

  const initialSignature = formSignature(form);
  let closing = false;

  const dirty = (): boolean => formSignature(form) !== initialSignature;

  const requestClose = async (): Promise<void> => {
    if (closing) return;
    closing = true;

    try {
      if (
        dirty() &&
        !(await requestConfirmation(
          "Discard the unapplied changes in this plugin form?",
          {
            eyebrow: "Unsaved changes",
            title: "Discard changes?",
            confirmLabel: "Discard changes",
            cancelLabel: "Keep editing",
            danger: true,
          },
        ))
      ) {
        return;
      }

      cancel.setAttribute(closeBypassAttribute, "");
      cancel.click();
    } finally {
      closing = false;
    }
  };

  cancel.addEventListener(
    "click",
    (event: MouseEvent) => {
      if (cancel.hasAttribute(closeBypassAttribute)) {
        cancel.removeAttribute(closeBypassAttribute);
        return;
      }

      event.preventDefault();
      event.stopImmediatePropagation();
      void requestClose();
    },
    { capture: true },
  );

  close.addEventListener("click", () => void requestClose());

  popover.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key !== "Escape") return;

    event.preventDefault();
    event.stopImmediatePropagation();
    void requestClose();
  });
}

// enhanceVisiblePopovers upgrades all plugin forms currently mounted in the page.
function enhanceVisiblePopovers(): void {
  for (const popover of document.querySelectorAll<HTMLElement>(
    ".visual-widget-popover",
  )) {
    enhancePopover(popover);
  }
}

// Initializes sticky, closable plugin forms with dirty-state protection.
export function initVisualWidgetPopovers(): void {
  if (typeof document === "undefined" || !document.body) return;

  enhanceVisiblePopovers();

  const observer = new MutationObserver(() => enhanceVisiblePopovers());
  observer.observe(document.body, { childList: true, subtree: true });
}
