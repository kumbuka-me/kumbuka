// Searchable visual-editor picker for resource-backed plugin completions.

import { errorMessage, responseProblem } from "../../core/http.ts";
import {
  invalidateEditorCatalog,
  loadEditorCatalog,
  type CatalogCompletion,
  type CatalogCompletionField,
  type CatalogCompletionProvider,
  type EditorCatalog,
} from "./catalog.ts";
import type { EditorInsertAction } from "./toolbar.ts";

let pickerID = 0;

// completionProviderForInsert resolves an insert action to one unambiguous completion provider.
export function completionProviderForInsert(
  catalog: EditorCatalog,
  insert: EditorInsertAction,
): CatalogCompletionProvider | undefined {
  if (
    !insert.pluginID ||
    (insert.mode && insert.mode !== "insert") ||
    insert.suffix
  )
    return undefined;

  const matches = catalog.completion_providers.filter(
    (provider) =>
      provider.plugin_id === insert.pluginID &&
      provider.trigger === insert.markdown,
  );
  return matches.length === 1 ? matches[0] : undefined;
}

// completionItemsForProvider returns concrete items owned by one completion provider.
export function completionItemsForProvider(
  catalog: EditorCatalog,
  provider: CatalogCompletionProvider,
): CatalogCompletion[] {
  return catalog.completions.filter(
    (item) =>
      item.plugin_id === provider.plugin_id &&
      item.module_id === provider.module_id,
  );
}

// filterCompletionItems applies case-insensitive label and detail filtering.
export function filterCompletionItems(
  items: CatalogCompletion[],
  query: string,
): CatalogCompletion[] {
  const normalized = query.trim().toLocaleLowerCase();
  if (!normalized) return items;

  return items.filter((item) =>
    `${item.label}\n${item.detail ?? ""}`
      .toLocaleLowerCase()
      .includes(normalized),
  );
}

// appendText creates a child element with optional class and text content.
function appendText<K extends keyof HTMLElementTagNameMap>(
  parent: HTMLElement,
  tag: K,
  text: string,
  className = "",
): HTMLElementTagNameMap[K] {
  const element = document.createElement(tag);
  if (className) element.className = className;
  element.textContent = text;
  parent.append(element);
  return element;
}

// fieldControl creates one generic control for a directly creatable resource field.
function fieldControl(field: CatalogCompletionField): HTMLElement {
  let control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

  switch (field.type) {
    case "textarea":
      control = document.createElement("textarea");
      control.value = field.default ?? "";
      break;
    case "select": {
      const select = document.createElement("select");
      if (!field.required) select.append(new Option("", ""));
      for (const option of field.options ?? []) {
        select.append(
          new Option(option, option, false, option === field.default),
        );
      }
      control = select;
      break;
    }
    case "boolean": {
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = field.default === "true";
      control = checkbox;
      break;
    }
    case "color": {
      const color = document.createElement("input");
      color.type = "color";
      color.value = field.default || "#64748b";
      control = color;
      break;
    }
    default: {
      const input = document.createElement("input");
      input.type = field.type === "url" ? "url" : "text";
      input.value = field.default ?? "";
      control = input;
      break;
    }
  }

  control.name = `resource_${field.id}`;
  control.required = field.required;
  return control;
}

// createResourceForm builds the direct-create form from host-validated resource metadata.
function createResourceForm(
  provider: CatalogCompletionProvider,
  insertName: string,
): HTMLFormElement {
  const form = document.createElement("form");
  form.className = "stacked-form editor-completion-create";
  form.hidden = true;

  const heading = appendText(
    form,
    "div",
    "",
    "editor-completion-create-heading",
  );
  appendText(heading, "strong", `New ${insertName}`);
  appendText(heading, "small", "Create it here without leaving the editor.");

  const resourceID = document.createElement("input");
  resourceID.type = "hidden";
  resourceID.name = "resource_id";
  resourceID.value = provider.resource_id;
  form.append(resourceID);

  for (const field of provider.fields) {
    const label = document.createElement("label");
    if (field.type === "boolean") label.className = "toggle-row";

    const labelText = document.createElement("span");
    labelText.textContent = field.name;
    if (field.required) {
      const required = document.createElement("small");
      required.textContent = " required";
      labelText.append(required);
    }
    label.append(labelText, fieldControl(field));
    form.append(label);
  }

  const error = appendText(form, "p", "", "editor-completion-create-error");
  error.hidden = true;
  error.setAttribute("role", "alert");

  const actions = appendText(
    form,
    "div",
    "",
    "editor-completion-create-actions",
  );
  const back = appendText(actions, "button", "Back", "button");
  back.type = "button";
  back.dataset.completionCreateBack = "";
  const save = appendText(
    actions,
    "button",
    `Add ${insertName}`,
    "button primary",
  );
  save.type = "submit";

  return form;
}

// createPickerDialog builds a fresh dialog so every open reflects the current catalog.
function createPickerDialog(
  provider: CatalogCompletionProvider,
  insert: EditorInsertAction,
): {
  dialog: HTMLDialogElement;
  search: HTMLInputElement;
  body: HTMLElement;
  options: HTMLElement;
  empty: HTMLElement;
  createButton?: HTMLButtonElement;
  createForm?: HTMLFormElement;
} {
  const insertName = insert.name || provider.resource_name || "item";
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog editor-completion-dialog";

  const header = appendText(dialog, "div", "", "app-dialog-header");
  const heading = appendText(header, "div", "");
  appendText(heading, "p", "Insert", "eyebrow");
  const title = appendText(heading, "h2", `Choose ${insertName}`);
  pickerID += 1;
  const titleID = `editor-completion-title-${pickerID}`;
  title.id = titleID;
  dialog.setAttribute("aria-labelledby", titleID);

  const close = appendText(
    header,
    "button",
    "×",
    "icon-button app-dialog-close",
  );
  close.type = "button";
  close.setAttribute("aria-label", `Close ${insertName} picker`);
  close.addEventListener("click", () => dialog.close());

  const searchWrap = appendText(dialog, "div", "", "editor-completion-search");
  const search = document.createElement("input");
  search.type = "search";
  search.placeholder = `Search ${provider.resource_name.toLocaleLowerCase()}…`;
  search.autocomplete = "off";
  search.setAttribute("aria-label", `Search ${provider.resource_name}`);
  searchWrap.append(search);

  const body = appendText(dialog, "div", "", "editor-completion-body");
  const options = appendText(body, "div", "", "editor-completion-options");
  const empty = appendText(
    body,
    "p",
    `No ${provider.resource_name.toLocaleLowerCase()} found.`,
    "editor-completion-empty muted",
  );
  empty.hidden = true;

  let createButton: HTMLButtonElement | undefined;
  let createForm: HTMLFormElement | undefined;
  if (
    provider.can_create &&
    provider.fields.length > 0 &&
    provider.resource_id
  ) {
    const actions = appendText(dialog, "div", "", "app-dialog-actions");
    createButton = appendText(
      actions,
      "button",
      `New ${insertName}`,
      "button primary",
    );
    createButton.type = "button";
    createForm = createResourceForm(provider, insertName);
    dialog.insertBefore(createForm, actions);
  }

  return { dialog, search, body, options, empty, createButton, createForm };
}

// renderCompletionOptions redraws the filtered completion choices.
function renderCompletionOptions(
  container: HTMLElement,
  empty: HTMLElement,
  items: CatalogCompletion[],
  onChoose: (replacement: string) => void,
): void {
  container.replaceChildren();
  empty.hidden = items.length > 0;

  for (const item of items) {
    const option = appendText(
      container,
      "button",
      "",
      "editor-completion-option",
    );
    option.type = "button";
    appendText(option, "strong", item.label);
    if (item.detail) appendText(option, "small", item.detail);
    option.addEventListener("click", () => onChoose(item.replacement));
  }
}

// formLabelValue reads the submitted value used as the completion display label.
function formLabelValue(
  form: HTMLFormElement,
  provider: CatalogCompletionProvider,
): string {
  const data = new FormData(form);
  return String(data.get(`resource_${provider.label_field}`) ?? "").trim();
}

// saveCompletionResource persists one new resource and returns the refreshed completion item.
async function saveCompletionResource(
  form: HTMLFormElement,
  provider: CatalogCompletionProvider,
  previous: CatalogCompletion[],
): Promise<CatalogCompletion | undefined> {
  const label = formLabelValue(form, provider);
  const response = await fetch(
    `/admin/plugin-settings/${encodeURIComponent(provider.plugin_id)}/resource-save`,
    {
      method: "POST",
      credentials: "same-origin",
      headers: { Accept: "application/json" },
      body: new FormData(form),
    },
  );
  if (!response.ok) throw await responseProblem(response);

  const previousReplacements = new Set(
    previous.map((item) => item.replacement),
  );
  invalidateEditorCatalog();
  const catalog = await loadEditorCatalog();
  const refreshedProvider = catalog.completion_providers.find(
    (candidate) =>
      candidate.plugin_id === provider.plugin_id &&
      candidate.module_id === provider.module_id,
  );
  if (!refreshedProvider) return undefined;

  const refreshed = completionItemsForProvider(catalog, refreshedProvider);
  if (label) {
    const matchingLabel = refreshed.find((item) => item.label.trim() === label);
    if (matchingLabel) return matchingLabel;
  }
  return refreshed.find((item) => !previousReplacements.has(item.replacement));
}

// openCompletionPicker opens a searchable resource picker when an insert has a matching provider.
export async function openCompletionPicker(
  insert: EditorInsertAction,
  onChoose: (replacement: string) => void,
): Promise<boolean> {
  let catalog: EditorCatalog;
  try {
    catalog = await loadEditorCatalog();
  } catch (error) {
    console.error("Could not load editor completions", error);
    return false;
  }

  const provider = completionProviderForInsert(catalog, insert);
  if (!provider) return false;

  let items = completionItemsForProvider(catalog, provider);
  const { dialog, search, body, options, empty, createButton, createForm } =
    createPickerDialog(provider, insert);
  document.body.append(dialog);

  const choose = (replacement: string): void => {
    dialog.close();
    onChoose(replacement);
  };
  const render = (): void => {
    empty.textContent = items.length
      ? `No matching ${provider.resource_name.toLocaleLowerCase()}.`
      : `No ${provider.resource_name.toLocaleLowerCase()} yet.`;
    renderCompletionOptions(
      options,
      empty,
      filterCompletionItems(items, search.value),
      choose,
    );
  };

  search.addEventListener("input", render);
  dialog.addEventListener("close", () => dialog.remove(), { once: true });

  if (createButton && createForm) {
    const error = createForm.querySelector<HTMLElement>(
      ".editor-completion-create-error",
    );
    createButton.addEventListener("click", () => {
      search.closest<HTMLElement>(".editor-completion-search")!.hidden = true;
      body.hidden = true;
      createButton.closest<HTMLElement>(".app-dialog-actions")!.hidden = true;
      createForm.hidden = false;
      createForm
        .querySelector<HTMLElement>(
          'input:not([type="hidden"]), textarea, select',
        )
        ?.focus();
    });
    createForm
      .querySelector<HTMLElement>("[data-completion-create-back]")
      ?.addEventListener("click", () => {
        createForm.hidden = true;
        createButton.closest<HTMLElement>(".app-dialog-actions")!.hidden =
          false;
        search.closest<HTMLElement>(".editor-completion-search")!.hidden =
          false;
        body.hidden = false;
        render();
        search.focus();
      });
    createForm.addEventListener("submit", (event) => {
      event.preventDefault();
      if (error) error.hidden = true;
      const submit = createForm.querySelector<HTMLButtonElement>(
        'button[type="submit"]',
      );
      if (submit) submit.disabled = true;

      void saveCompletionResource(createForm, provider, items)
        .then((created) => {
          if (!created)
            throw new Error("The new item was saved but could not be loaded.");
          items = [...items, created];
          choose(created.replacement);
        })
        .catch((saveError) => {
          if (!error) return;
          error.textContent = errorMessage(saveError);
          error.hidden = false;
        })
        .finally(() => {
          if (submit) submit.disabled = false;
        });
    });
  }

  render();
  dialog.showModal();
  search.focus();
  return true;
}
