// Custom Markdown source dialog shared by visual fallback and plugin widget editors.

export interface SourceDialogOptions {
  title: string;
  source: string;
  validate?: (source: string) => string;
}

// openSourceDialog opens an application-styled source editor and resolves the accepted source.
export function openSourceDialog(
  options: SourceDialogOptions,
): Promise<string | null> {
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog visual-source-dialog";
  dialog.setAttribute("aria-label", options.title);

  const form = document.createElement("form");
  form.className = "visual-source-dialog-form";

  const header = document.createElement("div");
  header.className = "app-dialog-header";
  const heading = document.createElement("div");
  const eyebrow = document.createElement("p");
  eyebrow.className = "eyebrow";
  eyebrow.textContent = "Visual editor";
  const title = document.createElement("h2");
  title.textContent = options.title;
  heading.append(eyebrow, title);

  const close = document.createElement("button");
  close.type = "button";
  close.className = "icon-button app-dialog-close";
  close.setAttribute("aria-label", "Close source editor");
  close.textContent = "×";
  header.append(heading, close);

  const body = document.createElement("div");
  body.className = "app-dialog-body visual-source-dialog-body";
  const label = document.createElement("label");
  label.className = "visual-source-dialog-field";
  const labelText = document.createElement("span");
  labelText.textContent = "Markdown source";
  const textarea = document.createElement("textarea");
  textarea.value = options.source;
  textarea.spellcheck = false;
  textarea.setAttribute("aria-label", "Markdown source");
  label.append(labelText, textarea);

  const error = document.createElement("p");
  error.className = "visual-source-dialog-error";
  error.setAttribute("role", "alert");
  error.hidden = true;
  body.append(label, error);

  const actions = document.createElement("div");
  actions.className = "app-dialog-actions";
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.className = "button";
  cancel.textContent = "Cancel";
  const apply = document.createElement("button");
  apply.type = "submit";
  apply.className = "button primary";
  apply.textContent = "Apply";
  actions.append(cancel, apply);

  form.append(header, body, actions);
  dialog.append(form);
  document.body.append(dialog);

  return new Promise<string | null>((resolve) => {
    let settled = false;

    // finish closes the dialog exactly once and resolves the caller result.
    const finish = (source: string | null): void => {
      if (settled) return;
      settled = true;
      if (dialog.open) dialog.close();
      dialog.remove();
      resolve(source);
    };

    const cancelDialog = (): void => finish(null);
    close.addEventListener("click", cancelDialog);
    cancel.addEventListener("click", cancelDialog);
    dialog.addEventListener("cancel", (event) => {
      event.preventDefault();
      cancelDialog();
    });
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) cancelDialog();
    });
    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const source = textarea.value;
      const problem = options.validate?.(source) ?? "";
      if (problem) {
        error.textContent = problem;
        error.hidden = false;
        textarea.focus();
        return;
      }
      finish(source);
    });
    textarea.addEventListener("input", () => {
      error.hidden = true;
    });

    dialog.showModal();
    textarea.focus();
    textarea.setSelectionRange(textarea.value.length, textarea.value.length);
  });
}
