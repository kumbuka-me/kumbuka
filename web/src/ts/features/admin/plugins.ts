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

// initAdminPlugins initializes plugin package pickers on administration pages.
export function initAdminPlugins(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-plugin-upload]",
  )) {
    setupPluginUpload(form);
  }

  setupPluginDialogs();
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
