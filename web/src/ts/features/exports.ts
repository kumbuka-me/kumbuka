// Page export and share-dialog behavior.

import { copyText } from "../core/clipboard.ts";
import { showNotice } from "../core/dialogs.ts";
import { requiredAttribute, requiredElement } from "../core/dom.ts";
import { errorMessage, requestJSON, responseProblem } from "../core/http.ts";
import { loadPrintPreview, parseExportPreview } from "./export-preview.ts";

export interface ExportParameterField {
  pluginID: string;
  moduleID: string;
  key: string;
  value: string;
  saved: string;
}

// Builds nested request-local plugin export parameters while omitting unchanged values.
export function exportParameterOverrides(
  fields: Iterable<ExportParameterField>,
): Record<string, Record<string, Record<string, string>>> {
  const plugins = new Map<string, Map<string, Map<string, string>>>();
  for (const field of fields) {
    if (field.value === field.saved) continue;
    let modules = plugins.get(field.pluginID);
    if (!modules) {
      modules = new Map();
      plugins.set(field.pluginID, modules);
    }
    let values = modules.get(field.moduleID);
    if (!values) {
      values = new Map();
      modules.set(field.moduleID, values);
    }
    values.set(field.key, field.value);
  }
  return Object.fromEntries(
    [...plugins].map(([pluginID, modules]) => [
      pluginID,
      Object.fromEntries(
        [...modules].map(([moduleID, values]) => [
          moduleID,
          Object.fromEntries(values),
        ]),
      ),
    ]),
  );
}

// Wires admin export behavior.
function setupAdminExport(): void {
  const exportForm =
    document.querySelector<HTMLFormElement>("[data-export-form]");
  const exportAll =
    exportForm?.querySelector<HTMLInputElement>("[data-export-all]");
  const exportPageCheckboxes = [
    ...(exportForm?.querySelectorAll<HTMLInputElement>('input[name="slug"]') ??
      []),
  ];

  exportAll?.addEventListener("change", () => {
    for (const checkbox of exportPageCheckboxes)
      checkbox.checked = exportAll.checked;
  });

  for (const checkbox of exportPageCheckboxes) {
    checkbox.addEventListener("change", () => {
      if (exportAll)
        exportAll.checked =
          exportPageCheckboxes.length > 0 &&
          exportPageCheckboxes.every((item) => item.checked);
    });
  }

  exportForm?.addEventListener("submit", async (event: SubmitEvent) => {
    if (
      event.submitter instanceof HTMLButtonElement &&
      event.submitter.name === "all"
    )
      return;
    if (exportPageCheckboxes.some((checkbox) => checkbox.checked)) return;

    event.preventDefault();
    await showNotice("Select at least one page to export.", {
      title: "Nothing selected",
    });
  });
}

// Starts a browser download for a response blob.
function downloadBlob(blob: Blob, filename: string): void {
  const objectURL = URL.createObjectURL(blob);
  const link = document.createElement("a");

  link.href = objectURL;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
}

// Reads a download filename from response headers.
function downloadFilename(response: Response, fallback: string): string {
  const disposition = response.headers.get("Content-Disposition") || "";
  const encoded = disposition.match(/filename\*=UTF-8''([^;]+)/i);
  if (encoded?.[1]) return decodeURIComponent(encoded[1]);

  const plain = disposition.match(/filename="?([^";]+)"?/i);

  return plain?.[1] || fallback;
}

// Wires share dialog behavior.
function setupShareDialog(dialog: HTMLDialogElement): void {
  const open = requiredElement<HTMLButtonElement>(
    document,
    "[data-share-dialog-open]",
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-share-dialog-close]",
  );
  const permalink = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-share-permalink]",
  );
  const permalinkStatus = requiredElement<HTMLElement>(
    dialog,
    "[data-share-permalink-status]",
  );
  const print = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-share-print]",
  );
  const pdf = requiredElement<HTMLButtonElement>(dialog, "[data-share-pdf]");
  const markdown = requiredElement<HTMLAnchorElement>(
    dialog,
    "[data-share-markdown]",
  );
  const progress = requiredElement<HTMLElement>(
    dialog,
    "[data-share-progress]",
  );
  const preview = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-share-preview]",
  );
  const previewPanel = requiredElement<HTMLElement>(
    dialog,
    "[data-export-preview-panel]",
  );
  const previewHost = requiredElement<HTMLElement>(
    dialog,
    "[data-export-preview-host]",
  );
  const previewHide = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-export-preview-hide]",
  );
  const errorOutput = requiredElement<HTMLElement>(
    dialog,
    "[data-share-error]",
  );
  const status = requiredElement<HTMLElement>(dialog, "[data-export-status]");
  const parameterForm = dialog.querySelector<HTMLFormElement>(
    "[data-export-parameters-form]",
  );
  const parameterDetails = dialog.querySelector<HTMLDetailsElement>(
    "[data-export-parameters]",
  );
  const fields = [
    ...dialog.querySelectorAll<HTMLTextAreaElement>("[data-export-key]"),
  ];
  const permalinkPath = requiredAttribute(permalink, "data-url");
  const pdfURL = requiredAttribute(pdf, "data-url");
  const previewURL = requiredAttribute(dialog, "data-preview-url");
  let operation: AbortController | null = null;
  let previewFrame: HTMLIFrameElement | null = null;
  let previewKey = "";

  function payload(): string {
    const parameters = exportParameterOverrides(
      fields.map((field) => ({
        pluginID: requiredAttribute(field, "data-export-plugin"),
        moduleID: requiredAttribute(field, "data-export-module"),
        key: requiredAttribute(field, "data-export-key"),
        value: field.value,
        saved: field.defaultValue,
      })),
    );
    return JSON.stringify({ parameters });
  }

  function clearPreview(): void {
    previewFrame = null;
    previewKey = "";
    previewPanel.hidden = true;
    previewHost.replaceChildren();
    dialog.classList.remove("share-dialog-preview");
  }

  function setBusy(busy: boolean): void {
    for (const control of [preview, print, pdf, ...fields])
      control.disabled = busy;
    const reset = parameterForm?.querySelector<HTMLButtonElement>(
      "[data-export-reset]",
    );
    if (reset) reset.disabled = busy;
    previewHide.disabled = busy;
    dialog.setAttribute("aria-busy", String(busy));
  }

  function resetSession(): void {
    operation?.abort();
    operation = null;
    parameterForm?.reset();
    if (parameterDetails) parameterDetails.open = false;
    clearPreview();
    errorOutput.textContent = "";
    errorOutput.hidden = true;
    status.hidden = true;
    progress.hidden = true;
    setBusy(false);
  }

  async function runExport(kind: "preview" | "print" | "pdf"): Promise<void> {
    if (operation) return;
    const controller = new AbortController();
    operation = controller;
    const body = payload(); // Snapshot input before awaiting any request.
    setBusy(true);
    errorOutput.textContent = "";
    errorOutput.hidden = true;
    status.textContent =
      kind === "print" ? "Preparing print document..." : "Preparing preview...";
    status.hidden = kind === "pdf";
    const timer =
      kind === "pdf"
        ? window.setTimeout(() => {
            if (operation === controller) progress.hidden = false;
          }, 350)
        : undefined;
    try {
      const init: RequestInit = {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Accept: kind === "pdf" ? "application/pdf" : "application/json",
        },
        body,
        signal: controller.signal,
        cache: "no-store",
        credentials: "same-origin",
      };
      if (kind === "pdf") {
        const response = await fetch(pdfURL, init);
        if (!response.ok) throw await responseProblem(response);
        const contentType = response.headers
          .get("Content-Type")
          ?.split(";")[0]
          .trim();
        if (contentType !== "application/pdf")
          throw new Error(
            "The server did not return a PDF. Your session may have expired.",
          );
        const blob = await response.blob();
        if (controller.signal.aborted) return;
        downloadBlob(blob, downloadFilename(response, "kumbuka-page.pdf"));
        dialog.close();
        return;
      }
      if (!previewFrame || previewKey !== body) {
        const documentHTML = parseExportPreview(
          await requestJSON(previewURL, init),
        );
        if (controller.signal.aborted) return;
        clearPreview();
        previewPanel.hidden = false;
        dialog.classList.add("share-dialog-preview");
        previewFrame = await loadPrintPreview(
          previewHost,
          documentHTML,
          controller.signal,
        );
        previewKey = body;
      }
      if (controller.signal.aborted) return;
      previewPanel.hidden = false;
      dialog.classList.add("share-dialog-preview");
      if (kind === "print") {
        const printWindow = previewFrame.contentWindow;
        if (!printWindow)
          throw new Error("The print document is not available.");
        printWindow.focus();
        printWindow.print();
        // Keep the frame alive: some browsers return before their print dialog
        // closes. Only closing this export session discards the temporary data.
      } else {
        previewPanel.scrollIntoView({ block: "nearest" });
      }
    } catch (error) {
      if (!controller.signal.aborted) {
        errorOutput.textContent = errorMessage(error);
        errorOutput.hidden = false;
        if (kind !== "pdf") clearPreview();
      }
    } finally {
      if (timer !== undefined) clearTimeout(timer);
      if (operation === controller) {
        operation = null;
        setBusy(false);
        progress.hidden = true;
        status.hidden = true;
      }
    }
  }

  open.addEventListener("click", () => {
    resetSession();
    dialog.showModal();
  });
  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", resetSession);
  dialog.addEventListener("keydown", (event: KeyboardEvent) => {
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "p") {
      event.preventDefault();
      void runExport("print");
    }
  });
  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) dialog.close();
  });
  parameterForm?.addEventListener("submit", (event: SubmitEvent) =>
    event.preventDefault(),
  );
  parameterForm?.addEventListener("reset", clearPreview);
  parameterForm?.addEventListener("input", () => {
    clearPreview();
    errorOutput.hidden = true;
  });
  previewHide.addEventListener("click", () => {
    clearPreview();
    preview.focus();
  });
  preview.addEventListener("click", () => void runExport("preview"));
  print.addEventListener("click", () => void runExport("print"));
  pdf.addEventListener("click", () => void runExport("pdf"));
  permalink.addEventListener("click", async () => {
    const original = permalinkStatus.textContent;
    try {
      const url = new URL(permalinkPath, window.location.href).href;

      await copyText(url);
      permalinkStatus.textContent = "Copied. Authentication is still required.";
      setTimeout(() => {
        permalinkStatus.textContent = original;
      }, 1800);
    } catch (error) {
      console.error("Permalink copy failed", error);
      dialog.close();
      await showNotice(
        errorMessage(error) || "Permalink could not be copied.",
        {
          title: "Copy failed",
        },
      );
    }
  });

  markdown.addEventListener("click", () => dialog.close());
}

// Initializes exports.
export function initExports(): void {
  setupAdminExport();

  const shareDialog = document.querySelector<HTMLDialogElement>(
    "[data-share-dialog]",
  );

  if (shareDialog) setupShareDialog(shareDialog);
}
