// Administrator authentication, application, and PDF configuration behavior.

import { requiredElement } from "../../core/dom.ts";
import { errorMessage, responseProblem } from "../../core/http.ts";

const sensitivePDFHeaderNames = new Set([
  "authorization",
  "cookie",
  "proxy-authorization",
  "x-api-key",
  "x-auth-token",
]);

// Formats one PDF byte size for the administrator test result.
function formatPDFSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;

  const kibibytes = bytes / 1024;
  if (kibibytes < 1024) {
    return `${kibibytes.toFixed(kibibytes < 10 ? 1 : 0)} KiB`;
  }

  return `${(kibibytes / 1024).toFixed(1)} MiB`;
}

type PDFTestControls = {
  form: HTMLFormElement;
  endpoint: HTMLInputElement;
  button: HTMLButtonElement;
  status: HTMLElement;
  dialog: HTMLDialogElement;
  preview: HTMLIFrameElement;
  openPDF: HTMLAnchorElement;
  pages: HTMLElement;
  pagesCheck: HTMLElement;
  pagesMark: HTMLElement;
  size: HTMLElement;
  previewURL: string;
};

function setPDFTestStatus(
  controls: PDFTestControls,
  state: "" | "testing" | "success" | "error",
  message: string,
): void {
  controls.status.dataset.state = state;
  controls.status.textContent = message;
}

function clearPDFTestPreview(controls: PDFTestControls): void {
  controls.preview.removeAttribute("src");
  controls.openPDF.removeAttribute("href");

  if (controls.previewURL) URL.revokeObjectURL(controls.previewURL);
  controls.previewURL = "";
}

function showPDFTestResult(
  controls: PDFTestControls,
  blob: Blob,
  pageCount: number,
  byteSize: number,
): void {
  clearPDFTestPreview(controls);

  controls.previewURL = URL.createObjectURL(blob);
  controls.preview.src = controls.previewURL;
  controls.openPDF.href = controls.previewURL;
  controls.size.textContent = formatPDFSize(byteSize);

  if (pageCount === 2) {
    controls.pagesCheck.dataset.state = "success";
    controls.pagesMark.textContent = "✓";
    controls.pages.textContent = "2 pages";
  } else if (pageCount > 0) {
    controls.pagesCheck.dataset.state = "warning";
    controls.pagesMark.textContent = "!";
    controls.pages.textContent = `${pageCount} page${pageCount === 1 ? "" : "s"} returned; expected 2`;
  } else {
    controls.pagesCheck.dataset.state = "warning";
    controls.pagesMark.textContent = "!";
    controls.pages.textContent = "Page count unavailable";
  }

  controls.dialog.showModal();
}

// Encodes the current PDF form, including unsaved dynamic request headers.
function pdfFormBody(form: HTMLFormElement): URLSearchParams {
  const body = new URLSearchParams();
  for (const [name, value] of new FormData(form)) {
    if (typeof value === "string") body.append(name, value);
  }

  return body;
}

async function testPDFEndpoint(controls: PDFTestControls): Promise<void> {
  const pdfURL = controls.endpoint.value.trim();
  if (!pdfURL) {
    setPDFTestStatus(controls, "error", "Enter a PDF service URL to test.");
    controls.endpoint.focus();
    return;
  }

  controls.button.disabled = true;
  setPDFTestStatus(
    controls,
    "testing",
    "Rendering the two-page PDF test document…",
  );

  try {
    const response = await fetch("/admin/pdf/test", {
      method: "POST",
      body: pdfFormBody(controls.form),
      credentials: "same-origin",
      headers: { Accept: "application/pdf" },
    });
    if (!response.ok) throw await responseProblem(response);

    const blob = await response.blob();
    const pageCount = Number.parseInt(
      response.headers.get("X-Kumbuka-PDF-Pages") ?? "0",
      10,
    );
    const reportedSize = Number.parseInt(
      response.headers.get("X-Kumbuka-PDF-Size") ?? "0",
      10,
    );
    const byteSize = reportedSize > 0 ? reportedSize : blob.size;

    showPDFTestResult(
      controls,
      blob,
      Number.isFinite(pageCount) ? pageCount : 0,
      byteSize,
    );
    setPDFTestStatus(
      controls,
      "success",
      "PDF service is working. Review the generated test document.",
    );
  } catch (error: unknown) {
    setPDFTestStatus(controls, "error", errorMessage(error));
  } finally {
    controls.button.disabled = false;
  }
}

// Assigns server form names to a newly created request-header row.
function assignPDFHeaderRowNames(row: HTMLElement, token: string): void {
  const prefix = `pdf_header_${token}_`;
  const rowKey = requiredElement<HTMLInputElement>(
    row,
    "[data-pdf-header-row-key]",
  );
  const id = requiredElement<HTMLInputElement>(row, "[data-pdf-header-id]");
  const name = requiredElement<HTMLInputElement>(row, "[data-pdf-header-name]");
  const value = requiredElement<HTMLInputElement>(
    row,
    "[data-pdf-header-value]",
  );
  const sensitive = requiredElement<HTMLInputElement>(
    row,
    "[data-pdf-header-sensitive]",
  );

  rowKey.name = "pdf_header_row";
  rowKey.value = token;
  id.name = `${prefix}id`;
  name.name = `${prefix}name`;
  value.name = `${prefix}value`;
  sensitive.name = `${prefix}sensitive`;
}

async function revealPDFHeader(
  controls: PDFTestControls,
  row: HTMLElement,
  button: HTMLButtonElement,
): Promise<void> {
  const id = button.dataset.pdfHeaderId;
  if (!id) return;

  button.disabled = true;
  try {
    const response = await fetch(
      `/admin/pdf/headers/${encodeURIComponent(id)}/reveal`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (!response.ok) throw await responseProblem(response);

    const payload = (await response.json()) as { value?: unknown };
    if (typeof payload.value !== "string") {
      throw new Error("The server returned an invalid PDF header value.");
    }

    const value = requiredElement<HTMLInputElement>(
      row,
      "[data-pdf-header-value]",
    );
    value.type = "text";
    value.value = payload.value;
    value.focus();
    button.textContent = "Revealed";
    setPDFTestStatus(controls, "", "");
  } catch (error: unknown) {
    button.disabled = false;
    setPDFTestStatus(controls, "error", errorMessage(error));
  }
}

// Wires one configurable PDF request-header row.
function bindPDFHeaderRow(controls: PDFTestControls, row: HTMLElement): void {
  const name = requiredElement<HTMLInputElement>(row, "[data-pdf-header-name]");
  const value = requiredElement<HTMLInputElement>(
    row,
    "[data-pdf-header-value]",
  );
  const sensitive = requiredElement<HTMLInputElement>(
    row,
    "[data-pdf-header-sensitive]",
  );
  const remove = requiredElement<HTMLButtonElement>(
    row,
    "[data-pdf-header-remove]",
  );
  const reveal = row.querySelector<HTMLButtonElement>(
    "[data-pdf-header-reveal]",
  );

  const refreshSensitiveValue = (): void => {
    if (!reveal || reveal.textContent !== "Revealed") {
      value.type = sensitive.checked ? "password" : "text";
    }
  };

  sensitive.addEventListener("change", () => {
    sensitive.dataset.userSet = "true";
    refreshSensitiveValue();
    setPDFTestStatus(controls, "", "");
  });

  name.addEventListener("input", () => {
    if (
      row.dataset.pdfHeaderExisting !== "true" &&
      sensitive.dataset.userSet !== "true"
    ) {
      sensitive.checked = sensitivePDFHeaderNames.has(
        name.value.trim().toLowerCase(),
      );
      refreshSensitiveValue();
    }

    setPDFTestStatus(controls, "", "");
  });
  value.addEventListener("input", () => setPDFTestStatus(controls, "", ""));
  remove.addEventListener("click", () => {
    row.remove();
    setPDFTestStatus(controls, "", "");
  });
  reveal?.addEventListener("click", () => {
    void revealPDFHeader(controls, row, reveal);
  });
}

// Wires the PDF integration settings, request headers, and endpoint test.
function setupPDFSettings(): void {
  const form = document.querySelector<HTMLFormElement>("[data-pdf-settings]");
  if (!form) return;

  const endpoint = form.elements.namedItem("pdf_url");
  if (!(endpoint instanceof HTMLInputElement)) {
    throw new Error('Missing required form control: [name="pdf_url"]');
  }

  const button = requiredElement<HTMLButtonElement>(form, "[data-pdf-test]");
  const status = requiredElement<HTMLElement>(form, "[data-pdf-test-status]");
  const dialog = requiredElement<HTMLDialogElement>(
    document,
    "[data-pdf-test-dialog]",
  );
  const preview = requiredElement<HTMLIFrameElement>(
    dialog,
    "[data-pdf-test-preview]",
  );
  const openPDF = requiredElement<HTMLAnchorElement>(
    dialog,
    "[data-pdf-test-open]",
  );
  const pages = requiredElement<HTMLElement>(dialog, "[data-pdf-test-pages]");
  const pagesCheck = requiredElement<HTMLElement>(
    dialog,
    "[data-pdf-test-pages-check]",
  );
  const pagesMark = requiredElement<HTMLElement>(
    pagesCheck,
    ".pdf-test-check-mark",
  );
  const size = requiredElement<HTMLElement>(dialog, "[data-pdf-test-size]");
  const list = requiredElement<HTMLElement>(form, "[data-pdf-header-list]");
  const template = requiredElement<HTMLTemplateElement>(
    form,
    "[data-pdf-header-template]",
  );
  const addHeader = requiredElement<HTMLButtonElement>(
    form,
    "[data-pdf-header-add]",
  );
  const closeButtons = [
    ...dialog.querySelectorAll<HTMLButtonElement>("[data-pdf-test-close]"),
  ];

  const controls: PDFTestControls = {
    form,
    endpoint,
    button,
    status,
    dialog,
    preview,
    openPDF,
    pages,
    pagesCheck,
    pagesMark,
    size,
    previewURL: "",
  };

  list
    .querySelectorAll<HTMLElement>("[data-pdf-header-row]")
    .forEach((row) => bindPDFHeaderRow(controls, row));

  let nextHeaderRow = 1;
  addHeader.addEventListener("click", () => {
    const row = template.content.firstElementChild?.cloneNode(true) as
      HTMLElement | null | undefined;
    if (!row) return;

    assignPDFHeaderRowNames(row, `n${nextHeaderRow++}`);
    list.append(row);
    bindPDFHeaderRow(controls, row);
    requiredElement<HTMLInputElement>(row, "[data-pdf-header-name]").focus();
  });

  endpoint.addEventListener("input", () => setPDFTestStatus(controls, "", ""));
  for (const close of closeButtons) {
    close.addEventListener("click", () => dialog.close());
  }
  dialog.addEventListener("close", () => clearPDFTestPreview(controls));
  button.addEventListener("click", () => void testPDFEndpoint(controls));
}

// Wires ordered configurable external-link rows in application settings.
function setupExternalLinks(): void {
  const settings = document.querySelector<HTMLElement>(
    "[data-external-link-settings]",
  );
  if (!settings) return;

  const list = requiredElement<HTMLElement>(
    settings,
    "[data-external-link-list]",
  );
  const template = requiredElement<HTMLTemplateElement>(
    settings,
    "[data-external-link-template]",
  );
  const add = requiredElement<HTMLButtonElement>(
    settings,
    "[data-external-link-add]",
  );

  const bindRow = (row: HTMLElement): void => {
    requiredElement<HTMLButtonElement>(
      row,
      "[data-external-link-remove]",
    ).addEventListener("click", () => row.remove());
  };

  list
    .querySelectorAll<HTMLElement>("[data-external-link-row]")
    .forEach(bindRow);

  add.addEventListener("click", () => {
    const row = template.content.firstElementChild?.cloneNode(true) as
      HTMLElement | null | undefined;
    if (!row) return;

    list.append(row);
    bindRow(row);
    requiredElement<HTMLInputElement>(
      row,
      'input[name="external_link_label"]',
    ).focus();
  });
}

// Shows only the fields used by the selected browser authentication mode.
function setupAuthenticationSettings(): void {
  const form = document.querySelector<HTMLFormElement>("[data-auth-settings]");
  if (!form) return;

  const mode = form.querySelector<HTMLSelectElement>("[data-auth-mode]");
  const sections = [
    ...form.querySelectorAll<HTMLElement>("[data-auth-fields]"),
  ];

  const refresh = (): void => {
    const effectiveMode = mode?.value ?? form.dataset.authEffectiveMode ?? "";
    sections.forEach((section) => {
      section.hidden = section.dataset.authFields !== effectiveMode;
    });
  };

  mode?.addEventListener("change", refresh);
  refresh();

  const groupSync = form.querySelector<HTMLInputElement>(
    "[data-oidc-group-sync-toggle]",
  );
  const groupOptions = form.querySelector<HTMLElement>(
    "[data-oidc-group-sync-options]",
  );
  const mappingList = form.querySelector<HTMLElement>(
    "[data-oidc-group-mapping-list]",
  );
  const mappingTemplate = form.querySelector<HTMLTemplateElement>(
    "[data-oidc-group-mapping-template]",
  );
  const addMapping = form.querySelector<HTMLButtonElement>(
    "[data-oidc-group-mapping-add]",
  );

  // Removes one mapping row without changing the saved configuration yet.
  const bindMapping = (row: HTMLElement): void => {
    row
      .querySelector<HTMLButtonElement>("[data-oidc-group-mapping-remove]")
      ?.addEventListener("click", () => row.remove());
  };

  mappingList
    ?.querySelectorAll<HTMLElement>("[data-oidc-group-mapping-row]")
    .forEach(bindMapping);

  addMapping?.addEventListener("click", () => {
    const row = mappingTemplate?.content.firstElementChild?.cloneNode(true) as
      HTMLElement | null | undefined;
    if (!row || !mappingList) return;

    mappingList.append(row);
    bindMapping(row);
    row
      .querySelector<HTMLInputElement>('input[name="oidc_group_source"]')
      ?.focus();
  });

  // Keeps optional synchronization details out of the way while disabled.
  const refreshGroupSync = (): void => {
    if (groupOptions) groupOptions.hidden = !groupSync?.checked;
  };

  groupSync?.addEventListener("change", refreshGroupSync);
  refreshGroupSync();
}

// Initializes administrator configuration controls present on the current page.
export function initAdminConfiguration(): void {
  setupExternalLinks();
  setupAuthenticationSettings();
  setupPDFSettings();
}
