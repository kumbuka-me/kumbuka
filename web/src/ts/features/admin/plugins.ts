// Plugin package upload picker with drag-and-drop support.

import { requiredElement } from "../../core/dom.ts";

const maxPluginPackageBytes = 16 * 1024 * 1024;

// pluginPackageProblem returns a user-facing validation message for an invalid package file.
function pluginPackageProblem(file: File): string {
  if (!file.name.toLowerCase().endsWith(".kumbukaplugin")) {
    return "Choose a .kumbukaplugin package.";
  }
  if (file.size > maxPluginPackageBytes) {
    return "Plugin packages must be 16 MiB or smaller.";
  }
  return "";
}

// formatFileSize returns a compact binary file-size label for the selected package.
function formatFileSize(bytes: number): string {
  const mebibyte = 1024 * 1024;
  const kibibyte = 1024;

  if (bytes >= mebibyte) {
    const value = bytes / mebibyte;
    return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} MiB`;
  }

  return `${Math.max(1, Math.ceil(bytes / kibibyte))} KiB`;
}

// setupPluginUpload wires one install or upgrade package picker.
function setupPluginUpload(form: HTMLFormElement): void {
  const input = requiredElement<HTMLInputElement>(
    form,
    "[data-plugin-upload-input]",
  );
  const dropzone = requiredElement<HTMLElement>(
    form,
    "[data-plugin-upload-dropzone]",
  );
  const selection = requiredElement<HTMLElement>(
    form,
    "[data-plugin-upload-selection]",
  );
  const name = requiredElement<HTMLElement>(form, "[data-plugin-upload-name]");
  const size = requiredElement<HTMLElement>(form, "[data-plugin-upload-size]");
  const error = requiredElement<HTMLElement>(
    form,
    "[data-plugin-upload-error]",
  );
  const clear = requiredElement<HTMLButtonElement>(
    form,
    "[data-plugin-upload-clear]",
  );
  const submit = requiredElement<HTMLButtonElement>(
    form,
    "[data-plugin-upload-submit]",
  );

  // showProblem displays a package-selection validation error.
  const showProblem = (message: string): void => {
    error.textContent = message;
    error.hidden = !message;
    dropzone.classList.toggle("is-invalid", Boolean(message));
    if (message) input.setAttribute("aria-invalid", "true");
    else input.removeAttribute("aria-invalid");
  };

  // sync reflects the current file input in the custom picker UI.
  const sync = (): void => {
    const file = input.files?.[0];
    if (!file) {
      dropzone.hidden = false;
      selection.hidden = true;
      submit.disabled = true;
      name.textContent = "";
      size.textContent = "";
      return;
    }

    const problem = pluginPackageProblem(file);
    if (problem) {
      input.value = "";
      dropzone.hidden = false;
      selection.hidden = true;
      submit.disabled = true;
      showProblem(problem);
      return;
    }

    showProblem("");
    dropzone.hidden = true;
    selection.hidden = false;
    submit.disabled = false;
    name.textContent = file.name;
    size.textContent = formatFileSize(file.size);
  };

  // chooseDroppedFile transfers one dropped file into the real form input.
  const chooseDroppedFile = (files: FileList): void => {
    if (files.length !== 1) {
      input.value = "";
      sync();
      showProblem("Choose exactly one .kumbukaplugin package.");
      return;
    }

    const file = files[0];
    const problem = pluginPackageProblem(file);
    if (problem) {
      input.value = "";
      sync();
      showProblem(problem);
      return;
    }

    const transfer = new DataTransfer();
    transfer.items.add(file);
    input.files = transfer.files;
    sync();
  };

  input.addEventListener("change", () => {
    showProblem("");
    sync();
  });

  clear.addEventListener("click", () => {
    input.value = "";
    showProblem("");
    sync();
    input.focus();
  });

  for (const type of ["dragenter", "dragover"] as const) {
    dropzone.addEventListener(type, (event: DragEvent) => {
      if (!Array.from(event.dataTransfer?.types ?? []).includes("Files"))
        return;
      event.preventDefault();
      dropzone.classList.add("is-dragging");
    });
  }

  dropzone.addEventListener("dragleave", (event: DragEvent) => {
    const related = event.relatedTarget;
    if (related instanceof Node && dropzone.contains(related)) return;
    dropzone.classList.remove("is-dragging");
  });

  dropzone.addEventListener("drop", (event: DragEvent) => {
    event.preventDefault();
    dropzone.classList.remove("is-dragging");
    const files = event.dataTransfer?.files;
    if (files?.length) chooseDroppedFile(files);
  });

  form.addEventListener("submit", (event: SubmitEvent) => {
    if (input.files?.length === 1 && !pluginPackageProblem(input.files[0])) {
      return;
    }

    event.preventDefault();
    input.click();
  });

  sync();
}

// setupPluginUpdate shows immediate feedback while the blocking catalog update request is running.
function setupPluginUpdate(form: HTMLFormElement): void {
  const submit = requiredElement<HTMLButtonElement>(
    form,
    "[data-plugin-update-submit]",
  );
  const spinner = requiredElement<HTMLElement>(
    form,
    "[data-plugin-update-spinner]",
  );
  const label = requiredElement<HTMLElement>(
    form,
    "[data-plugin-update-label]",
  );

  let submitting = false;

  form.addEventListener("submit", (event: SubmitEvent) => {
    if (submitting) return;

    event.preventDefault();
    submitting = true;

    form.setAttribute("aria-busy", "true");
    submit.disabled = true;
    spinner.hidden = false;
    label.textContent =
      form.dataset.pluginUpdatePending || "Downloading update…";

    // Give the browser one paint before navigation starts so the pending state is visible.
    requestAnimationFrame(() => HTMLFormElement.prototype.submit.call(form));
  });
}

type PluginListControl = HTMLInputElement | HTMLSelectElement;
type PluginListRow = Record<string, string>;

const legacyPluginColors = new Map<string, string>([
  ["gray", "#64748b"],
  ["blue", "#2563eb"],
  ["green", "#16a34a"],
  ["yellow", "#ca8a04"],
  ["orange", "#ea580c"],
  ["red", "#dc2626"],
  ["purple", "#9333ea"],
  ["teal", "#0f766e"],
]);

// pluginListControls returns the editable column controls inside one list row.
function pluginListControls(root: ParentNode): PluginListControl[] {
  return [
    ...root.querySelectorAll<PluginListControl>("[data-plugin-list-column]"),
  ];
}

// parsePluginListRows decodes canonical JSON and the legacy pipe-delimited row format.
function parsePluginListRows(
  value: string,
  columns: PluginListControl[],
): PluginListRow[] {
  if (!value.trim()) return [];

  try {
    const parsed: unknown = JSON.parse(value);
    if (!Array.isArray(parsed)) return [];
    return parsed.flatMap((row): PluginListRow[] => {
      if (!row || typeof row !== "object" || Array.isArray(row)) return [];
      const record = row as Record<string, unknown>;
      const normalized: PluginListRow = {};
      for (const column of columns) {
        const id = column.dataset.pluginListColumn || "";
        const cell = record[id];
        if (!id || typeof cell !== "string") return [];
        normalized[id] = cell;
      }
      return [normalized];
    });
  } catch {
    const rows: PluginListRow[] = [];
    for (const rawLine of value.split("\n")) {
      const line = rawLine.trim();
      if (!line) continue;
      const cells = line.split("|").map((cell) => cell.trim());
      if (cells.length !== columns.length) return [];
      const row: PluginListRow = {};
      columns.forEach((column, index) => {
        const id = column.dataset.pluginListColumn || "";
        let cell = cells[index] || "";
        if (column instanceof HTMLInputElement && column.type === "color") {
          cell = legacyPluginColors.get(cell.toLowerCase()) || cell;
        }
        row[id] = cell;
      });
      rows.push(row);
    }
    return rows;
  }
}

// setupPluginListField turns one manifest list field into addable structured rows backed by canonical JSON.
function setupPluginListField(field: HTMLElement): void {
  const value = requiredElement<HTMLInputElement>(
    field,
    "[data-plugin-list-value]",
  );
  const rows = requiredElement<HTMLElement>(field, "[data-plugin-list-rows]");
  const template = requiredElement<HTMLTemplateElement>(
    field,
    "[data-plugin-list-template]",
  );
  const add = requiredElement<HTMLButtonElement>(
    field,
    "[data-plugin-list-add]",
  );
  const templateControls = pluginListControls(template.content);
  const maxItems = Math.max(
    1,
    Number.parseInt(field.dataset.maxItems || "16", 10) || 16,
  );

  field.style.setProperty(
    "--plugin-list-columns",
    String(Math.max(1, templateControls.length)),
  );

  // sync serializes the current rows and keeps server-side field errors attached to a visible control.
  const sync = (): void => {
    const result: PluginListRow[] = [];
    const rowElements = rows.querySelectorAll<HTMLElement>(
      "[data-plugin-list-row]",
    );
    for (const rowElement of rowElements) {
      const row: PluginListRow = {};
      for (const control of pluginListControls(rowElement)) {
        const id = control.dataset.pluginListColumn || "";
        if (id) row[id] = control.value;
      }
      result.push(row);
    }
    value.value = JSON.stringify(result);

    for (const control of field.querySelectorAll<PluginListControl>(
      "[data-error-field]",
    )) {
      delete control.dataset.errorField;
    }
    const first = pluginListControls(rows)[0];
    if (first) first.dataset.errorField = value.name;

    add.disabled = rowElements.length >= maxItems;
  };

  // appendRow creates one editable row from the declarative column template.
  const appendRow = (initial: PluginListRow = {}): void => {
    if (rows.childElementCount >= maxItems) return;
    const fragment = template.content.cloneNode(true) as DocumentFragment;
    const row = fragment.querySelector<HTMLElement>(".plugin-list-row");
    if (!row) return;
    row.dataset.pluginListRow = "true";

    for (const control of pluginListControls(row)) {
      const id = control.dataset.pluginListColumn || "";
      const fallback = control.dataset.pluginListDefault || control.value;
      control.value = initial[id] || fallback;
      control.addEventListener("input", sync);
      control.addEventListener("change", sync);
    }

    row
      .querySelector<HTMLButtonElement>("[data-plugin-list-remove]")
      ?.addEventListener("click", () => {
        row.remove();
        sync();
      });
    rows.append(row);
    sync();
  };

  const storedValue = value.value;
  const initialRows = parsePluginListRows(storedValue, templateControls);
  if (storedValue.trim() && initialRows.length === 0) {
    const error = document.createElement("small");
    error.className = "field-validation-error";
    error.setAttribute("role", "alert");
    error.textContent =
      "The saved list value is invalid. Re-enter the rows and save the record.";
    field.append(error);
  }
  for (const row of initialRows) appendRow(row);
  if (rows.childElementCount === 0) appendRow();

  add.addEventListener("click", () => appendRow());
  sync();
}

// initAdminPlugins initializes plugin package pickers on administration pages.
export function initAdminPlugins(): void {
  for (const field of document.querySelectorAll<HTMLElement>(
    "[data-plugin-list-field]",
  )) {
    setupPluginListField(field);
  }

  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-plugin-upload]",
  )) {
    setupPluginUpload(form);
  }

  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-plugin-update]",
  )) {
    setupPluginUpdate(form);
  }

  setupPluginToggles();
  setupPluginDialogs();
}

let pluginToggleSubmitBound = false;

// pluginStateControl returns one list or detail state control for a plugin.
function pluginStateControl(
  root: ParentNode,
  pluginID: string,
  surface: string,
): HTMLElement | null {
  return (
    [...root.querySelectorAll<HTMLElement>("[data-plugin-state-control]")].find(
      (control) =>
        control.dataset.pluginStateControl === pluginID &&
        control.dataset.pluginStateSurface === surface,
    ) ?? null
  );
}

// setPluginTogglePending prevents duplicate lifecycle requests for one plugin.
function setPluginTogglePending(pluginID: string, pending: boolean): void {
  for (const control of document.querySelectorAll<HTMLElement>(
    "[data-plugin-state-control]",
  )) {
    if (control.dataset.pluginStateControl !== pluginID) continue;

    if (pending) control.setAttribute("aria-busy", "true");
    else control.removeAttribute("aria-busy");

    for (const button of control.querySelectorAll<HTMLButtonElement>(
      "button[type='submit']",
    )) {
      button.disabled = pending;
    }
  }
}

// pluginToggleProblem returns the server-rendered lifecycle error, if present.
function pluginToggleProblem(result: Document): string {
  return (
    result.querySelector<HTMLElement>("[role='alert']")?.textContent?.trim() ||
    "Could not update the plugin state. Try again."
  );
}

// showPluginToggleProblem keeps a failed lifecycle action visible without navigating away.
function showPluginToggleProblem(form: HTMLFormElement, message: string): void {
  const container =
    form.closest<HTMLElement>(".plugin-detail-body") ??
    form.closest<HTMLElement>(".settings-panel");
  if (!container) return;

  let problem = container.querySelector<HTMLElement>(
    "[data-plugin-toggle-problem]",
  );
  if (!problem) {
    problem = document.createElement("p");
    problem.className = "settings-note";
    problem.dataset.pluginToggleProblem = "";
    problem.setAttribute("role", "alert");
    container.prepend(problem);
  }
  problem.textContent = message;
}

// clearPluginToggleProblem removes a previous lifecycle error from this surface.
function clearPluginToggleProblem(form: HTMLFormElement): void {
  const container =
    form.closest<HTMLElement>(".plugin-detail-body") ??
    form.closest<HTMLElement>(".settings-panel");
  container?.querySelector("[data-plugin-toggle-problem]")?.remove();
}

// syncPluginState replaces list and detail controls from the server-rendered result.
function syncPluginState(result: Document, pluginID: string): boolean {
  let replaced = false;

  for (const surface of ["list", "detail"]) {
    const current = pluginStateControl(document, pluginID, surface);
    const next = pluginStateControl(result, pluginID, surface);
    if (!current || !next) continue;

    current.replaceChildren(
      ...[...next.childNodes].map((node) => node.cloneNode(true)),
    );
    replaced = true;
  }

  return replaced;
}

// togglePlugin submits a lifecycle action without reloading the administration page.
async function togglePlugin(form: HTMLFormElement): Promise<void> {
  const pluginID = form.dataset.pluginToggle;
  const surface = form.closest<HTMLElement>("[data-plugin-state-control]")
    ?.dataset.pluginStateSurface;
  if (!pluginID || !surface) return;

  clearPluginToggleProblem(form);
  setPluginTogglePending(pluginID, true);

  try {
    const response = await fetch(form.action, {
      method: "POST",
      credentials: "same-origin",
      headers: { Accept: "text/html" },
    });
    const result = new DOMParser().parseFromString(
      await response.text(),
      "text/html",
    );

    if (!response.ok || !response.redirected) {
      showPluginToggleProblem(form, pluginToggleProblem(result));
      return;
    }
    if (!syncPluginState(result, pluginID)) {
      showPluginToggleProblem(
        form,
        "Plugin state changed, but the page could not refresh its controls.",
      );
      return;
    }

    pluginStateControl(document, pluginID, surface)
      ?.querySelector<HTMLButtonElement>("button[type='submit']")
      ?.focus({ preventScroll: true });
  } catch {
    showPluginToggleProblem(
      form,
      "Could not update the plugin state. Check your connection and try again.",
    );
  } finally {
    setPluginTogglePending(pluginID, false);
  }
}

// setupPluginToggles progressively enhances enable/disable forms without page navigation.
function setupPluginToggles(): void {
  if (pluginToggleSubmitBound) return;
  pluginToggleSubmitBound = true;

  document.addEventListener("submit", (event: SubmitEvent) => {
    const form = event.target;
    if (
      !(form instanceof HTMLFormElement) ||
      !form.matches("[data-plugin-toggle]")
    ) {
      return;
    }

    event.preventDefault();
    void togglePlugin(form);
  });
}

// pluginDialogs returns every server-rendered plugin detail dialog on the page.
function pluginDialogs(): HTMLDialogElement[] {
  return [
    ...document.querySelectorAll<HTMLDialogElement>(
      "[data-plugin-detail-dialog]",
    ),
  ];
}

// findPluginDialog resolves a plugin detail dialog without interpolating the ID into a selector.
function findPluginDialog(pluginID: string): HTMLDialogElement | null {
  return (
    pluginDialogs().find((dialog) => dialog.dataset.pluginId === pluginID) ??
    null
  );
}

// replacePluginURL keeps the address bar aligned with the currently open plugin modal.
function replacePluginURL(pluginID = ""): void {
  const url = new URL(window.location.href);
  url.pathname = "/admin/plugins";
  if (pluginID) url.searchParams.set("plugin", pluginID);
  else url.searchParams.delete("plugin");
  history.replaceState(null, "", `${url.pathname}${url.search}${url.hash}`);
}

// openPluginDialog opens one plugin and closes any other plugin detail dialog first.
function openPluginDialog(pluginID: string, updateURL = true): void {
  const dialog = findPluginDialog(pluginID);
  if (!dialog) return;

  for (const other of pluginDialogs()) {
    if (other !== dialog && other.open) other.close();
  }

  if (!dialog.open) dialog.showModal();
  if (updateURL) replacePluginURL(pluginID);
}

// openPluginFromElement resolves and opens the plugin associated with one list or dependency opener.
function openPluginFromElement(opener: HTMLElement): boolean {
  const pluginID = opener.dataset.pluginDetailOpen;
  if (!pluginID || !findPluginDialog(pluginID)) return false;

  openPluginDialog(pluginID);
  return true;
}

// pluginRowAction reports whether a row click belongs to an embedded control that must act independently.
function pluginRowAction(
  opener: HTMLElement,
  target: EventTarget | null,
): boolean {
  return (
    opener instanceof HTMLTableRowElement &&
    target instanceof Element &&
    Boolean(target.closest("button, input, select, textarea, form"))
  );
}

// setupPluginDialogs wires plugin rows and links, close controls, and server-requested modal state.
function setupPluginDialogs(): void {
  for (const opener of document.querySelectorAll<HTMLElement>(
    "[data-plugin-detail-open]",
  )) {
    opener.addEventListener("click", (event: MouseEvent) => {
      if (
        event.button !== 0 ||
        event.metaKey ||
        event.ctrlKey ||
        event.shiftKey ||
        event.altKey ||
        pluginRowAction(opener, event.target)
      ) {
        return;
      }

      if (!openPluginFromElement(opener)) return;
      event.preventDefault();
    });

    if (opener instanceof HTMLTableRowElement) {
      opener.addEventListener("keydown", (event: KeyboardEvent) => {
        if (
          event.target !== opener ||
          (event.key !== "Enter" && event.key !== " ")
        ) {
          return;
        }

        if (!openPluginFromElement(opener)) return;
        event.preventDefault();
      });
    }
  }

  for (const dialog of pluginDialogs()) {
    const pluginID = dialog.dataset.pluginId ?? "";

    for (const close of dialog.querySelectorAll<HTMLButtonElement>(
      "[data-plugin-detail-close]",
    )) {
      close.addEventListener("click", () => dialog.close());
    }

    dialog.addEventListener("click", (event: MouseEvent) => {
      if (event.target === dialog) dialog.close();
    });

    dialog.addEventListener("close", () => {
      const selected = new URL(window.location.href).searchParams.get("plugin");
      if (selected === pluginID) replacePluginURL();
    });
  }

  const initial = document.querySelector<HTMLDialogElement>(
    "[data-plugin-detail-open-on-load]",
  );
  if (initial?.dataset.pluginId) openPluginDialog(initial.dataset.pluginId);
}
