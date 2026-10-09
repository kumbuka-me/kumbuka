// Form controls for plugin-provided visual-editor widgets.

import type { CatalogCompletion } from "./catalog.ts";
import { openSourceDialog } from "./source-dialog.ts";
import {
  createMarkdownControl,
  disposeMarkdownControls,
  setMarkdownControlValue,
} from "./visual-widget-markdown.ts";
import { createLabel, setupMentionSetting } from "./visual-widget-controls.ts";
import {
  collectTreeValues,
  createTreeSetting,
  treeFormProblem,
} from "./visual-widget-tree.ts";
import {
  matchWidgetSource,
  normalizeWidgetColor,
  rewriteWidgetSource,
  splitWidgetList,
  validateWidgetValues,
  widgetAttribute,
  widgetValues,
  type CatalogWidget,
  type CatalogWidgetSetting,
} from "./widget-contract.ts";

function createScalarSetting(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
  completions: CatalogCompletion[],
): HTMLElement {
  const wrapper =
    setting.type === "markdown"
      ? document.createElement("div")
      : createLabel(setting.label);
  if (setting.type === "markdown") {
    wrapper.className = "visual-widget-field";
    const title = document.createElement("span");
    title.className = "visual-widget-field-label";
    title.textContent = setting.label;
    wrapper.append(title);
  }
  const attribute = widgetAttribute(widget, setting.attribute || "");
  if (!attribute) return wrapper;

  if (setting.type === "markdown") {
    const markdown = createMarkdownControl(
      values[attribute.name] || "",
      setting.label,
      setting.placeholder || "",
    );
    markdown.source.dataset.widgetAttribute = attribute.name;
    wrapper.append(markdown.element);
    return wrapper;
  }

  let control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
  if (setting.type === "select" || setting.type === "resource") {
    const select = document.createElement("select");
    if (setting.type === "resource") {
      const current = values[attribute.name] || "";
      const options = completions.filter(
        (item) =>
          item.plugin_id === widget.plugin_id &&
          item.module_id === setting.completion_module_id,
      );
      const seen = new Set<string>();
      if (!attribute.required || !current) {
        const option = document.createElement("option");
        option.value = "";
        option.textContent = setting.placeholder || "Choose…";
        select.append(option);
      }
      for (const item of options) {
        const value = widgetValues(item.replacement, widget)[attribute.name];
        if (!value || seen.has(value)) continue;
        seen.add(value);
        const option = document.createElement("option");
        option.value = value;
        option.textContent = item.detail
          ? `${item.label} — ${item.detail}`
          : item.label;
        select.append(option);
      }
      if (current && !seen.has(current)) {
        const option = document.createElement("option");
        option.value = current;
        option.textContent = current;
        select.append(option);
      }
    } else {
      if (!attribute.required && attribute.default === undefined) {
        const option = document.createElement("option");
        option.value = "";
        option.textContent = "Default";
        select.append(option);
      }
      for (const value of attribute.values || []) {
        const option = document.createElement("option");
        option.value = value;
        option.textContent = value;
        select.append(option);
      }
    }
    control = select;
  } else if (setting.type === "textarea") {
    const textarea = document.createElement("textarea");
    textarea.rows = 5;
    textarea.placeholder = setting.placeholder || "";
    control = textarea;
  } else {
    const input = document.createElement("input");
    input.type = setting.type === "date" ? "date" : "text";
    input.placeholder = setting.placeholder || "";
    if (setting.type === "mention") setupMentionSetting(input, wrapper);
    if (setting.suggestions?.length) {
      const list = document.createElement("datalist");
      list.id = `visual-widget-suggestions-${widget.id}-${attribute.name}`;
      for (const value of setting.suggestions) {
        const option = document.createElement("option");
        option.value = value;
        list.append(option);
      }
      input.setAttribute("list", list.id);
      wrapper.append(list);
    }
    control = input;
  }
  control.dataset.widgetAttribute = attribute.name;
  control.value = values[attribute.name] || "";
  if (attribute.required) control.required = true;
  wrapper.append(control);
  return wrapper;
}

function defaultPreviewColors(widget: CatalogWidget): string[] {
  return widget.preview.kind === "badge"
    ? widget.preview.badge.default_colors || []
    : [];
}

function listRows(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): string[][] {
  if (setting.row_separator && setting.attributes?.length === 1) {
    const name = setting.attributes[0];
    const attribute = widgetAttribute(widget, name);
    const items = attribute
      ? splitWidgetList(values[name] || "", attribute)
      : [];
    const count = setting.columns?.length || 0;
    const rows = items.map((item) => {
      const separator = item.indexOf(setting.row_separator || "");
      if (separator < 0) return [item, ...Array(count - 1).fill("")];
      return [
        item.slice(0, separator),
        item.slice(separator + setting.row_separator!.length),
        ...Array(Math.max(0, count - 2)).fill(""),
      ];
    });
    return rows.length ? rows : [Array(count).fill("")];
  }
  const lists = (setting.attributes || []).map((name) => {
    const attribute = widgetAttribute(widget, name);
    return attribute ? splitWidgetList(values[name] || "", attribute) : [];
  });
  const count = Math.max(1, ...lists.map((list) => list.length));
  const defaults = defaultPreviewColors(widget);
  return Array.from({ length: count }, (_, row) =>
    lists.map((list, column) => {
      if (list[row]) return list[row];
      const target = widgetAttribute(
        widget,
        setting.attributes?.[column] || "",
      );
      if (target?.type === "color-list")
        return defaults[row % Math.max(1, defaults.length)] || "#64748b";
      return "";
    }),
  );
}

function exactlyOneActivationAttribute(
  widget: CatalogWidget,
  attributes: string[],
): string {
  for (const constraint of widget.constraints || []) {
    if (constraint.kind !== "exactly-one") continue;
    const match = attributes.find((name) =>
      constraint.attributes.includes(name),
    );
    if (match) return match;
  }
  return "";
}

function appendTableRow(
  body: HTMLTableSectionElement,
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: string[],
): HTMLTableRowElement {
  const row = body.insertRow();
  const activationAttribute = exactlyOneActivationAttribute(
    widget,
    setting.attributes || [],
  );
  (setting.columns || []).forEach((column, index) => {
    const cell = row.insertCell();
    const attributeName = setting.row_separator
      ? setting.attributes?.[0] || ""
      : setting.attributes?.[index] || "";
    const attribute = widgetAttribute(widget, attributeName);
    const value =
      column.type === "color"
        ? normalizeWidgetColor(values[index] || "", attribute) || "#64748b"
        : values[index] || "";
    if (column.type === "markdown") {
      const markdown = createMarkdownControl(value, column.label, "");
      markdown.source.dataset.widgetTableAttribute = attributeName;
      markdown.source.dataset.widgetTableColumn = String(index);
      if (activationAttribute)
        markdown.source.dataset.widgetExclusiveAttribute = activationAttribute;
      cell.append(markdown.element);
      return;
    }
    const control =
      column.type === "textarea"
        ? document.createElement("textarea")
        : document.createElement("input");
    if (control instanceof HTMLInputElement) control.type = column.type;
    if (control instanceof HTMLTextAreaElement) control.rows = 3;
    control.value = value;
    control.dataset.widgetTableAttribute = attributeName;
    control.dataset.widgetTableColumn = String(index);
    if (activationAttribute)
      control.dataset.widgetExclusiveAttribute = activationAttribute;
    control.setAttribute("aria-label", column.label);
    cell.append(control);
  });
  const actions = row.insertCell();
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "visual-widget-remove-row";
  remove.textContent = "Remove";
  remove.addEventListener("click", () => {
    if (body.rows.length > 1) {
      disposeMarkdownControls(row);
      row.remove();
    } else
      row
        .querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
          "input, textarea",
        )
        .forEach((control) => {
          const value =
            control instanceof HTMLInputElement && control.type === "color"
              ? "#64748b"
              : "";
          if (control instanceof HTMLTextAreaElement)
            setMarkdownControlValue(control, value);
          else control.value = value;
        });
  });
  actions.append(remove);
  return row;
}

function createTableSetting(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
  activate: (attribute: string) => void,
): HTMLElement {
  const field = document.createElement("fieldset");
  field.className = "visual-widget-table-field";
  const legend = document.createElement("legend");
  legend.textContent = setting.label;
  field.append(legend);

  const table = document.createElement("table");
  table.className = "visual-widget-table";
  const head = table.createTHead().insertRow();
  for (const column of setting.columns || []) {
    const th = document.createElement("th");
    th.scope = "col";
    th.textContent = column.label;
    head.append(th);
  }
  const actionHead = document.createElement("th");
  actionHead.scope = "col";
  actionHead.textContent = "";
  head.append(actionHead);
  const body = table.createTBody();
  for (const row of listRows(setting, widget, values))
    appendTableRow(body, setting, widget, row);
  field.append(table);

  const add = document.createElement("button");
  add.type = "button";
  add.className = "visual-widget-add-row";
  add.textContent = "Add row";
  add.addEventListener("click", () => {
    const colors = defaultPreviewColors(widget);
    const rowIndex = body.rows.length;
    const row = appendTableRow(
      body,
      setting,
      widget,
      (setting.columns || []).map((column) =>
        column.type === "color"
          ? colors[rowIndex % Math.max(1, colors.length)] || "#64748b"
          : "",
      ),
    );
    activate(exactlyOneActivationAttribute(widget, setting.attributes || []));
    const editable = row.querySelector<HTMLElement>(
      '[contenteditable="true"], input[type="text"], input:not([type]), textarea:not([hidden])',
    );
    editable?.focus();
  });
  field.append(add);
  return field;
}

function tableRows(
  form: HTMLFormElement,
  setting: CatalogWidgetSetting,
): string[][] {
  const attributes = setting.attributes || [];
  const first = form.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `[data-widget-table-attribute="${CSS.escape(attributes[0] || "")}"]`,
  );
  const table = first?.closest("table");
  if (!table) return [];

  return [...(table.tBodies[0]?.rows || [])]
    .map((row) =>
      (setting.columns || []).map((_, index) =>
        (
          row.querySelector<HTMLInputElement | HTMLTextAreaElement>(
            `[data-widget-table-column="${index}"]`,
          )?.value || ""
        ).trim(),
      ),
    )
    .filter((row) => Boolean(row[0]));
}

function tableRowsIncludingEmpty(
  form: HTMLFormElement,
  setting: CatalogWidgetSetting,
): string[][] {
  const first = form.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `[data-widget-table-attribute="${CSS.escape(setting.attributes?.[0] || "")}"]`,
  );
  const table = first?.closest("table");
  if (!table) return [];
  return [...(table.tBodies[0]?.rows || [])].map((row) =>
    (setting.columns || []).map(
      (_, index) =>
        row
          .querySelector<HTMLInputElement | HTMLTextAreaElement>(
            `[data-widget-table-column="${index}"]`,
          )
          ?.value.trim() || "",
    ),
  );
}

function compoundTableProblem(
  form: HTMLFormElement,
  widget: CatalogWidget,
): string {
  for (const setting of widget.settings) {
    if (setting.type !== "table" || !setting.row_separator) continue;
    for (const row of tableRowsIncludingEmpty(form, setting)) {
      if (row.some(Boolean) && row.some((value) => !value))
        return `Complete every ${setting.label.toLocaleLowerCase()} row.`;
    }
  }
  if (widget.preview.kind !== "card") return "";
  const annotations = widget.preview.card.line_annotations;
  if (!annotations) return "";
  const setting = widget.settings.find(
    (candidate) =>
      candidate.type === "table" &&
      candidate.attributes?.[0] === annotations.attribute,
  );
  if (!setting) return "";
  for (const row of tableRowsIncludingEmpty(form, setting)) {
    if (!row[0]) continue;
    const match = /^(\d+)(?:-(\d+))?$/u.exec(row[0]);
    const start = Number(match?.[1]);
    const end = Number(match?.[2] || match?.[1]);
    if (!match || start < 1 || end < start)
      return "Use a positive line number or inclusive range such as 12-15.";
  }
  return "";
}

function defaultTableColors(widget: CatalogWidget, count: number): string[] {
  const colors = defaultPreviewColors(widget);
  return Array.from({ length: count }, (_, index) =>
    (colors[index % Math.max(1, colors.length)] || "#64748b").toLowerCase(),
  );
}

function resetTableSetting(
  form: HTMLFormElement,
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
): void {
  const name = setting.attributes?.[0] || "";
  const input = form.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `[data-widget-table-attribute="${CSS.escape(name)}"]`,
  );
  const body = input?.closest("table")?.tBodies[0];
  if (!body) return;

  disposeMarkdownControls(body);
  body.replaceChildren();
  appendTableRow(
    body,
    setting,
    widget,
    (setting.columns || []).map((column) =>
      column.type === "color"
        ? defaultPreviewColors(widget)[0] || "#64748b"
        : "",
    ),
  );
}

function clearFormAttribute(
  form: HTMLFormElement,
  widget: CatalogWidget,
  attribute: string,
): void {
  const scalar = form.querySelector<
    HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  >(`[data-widget-attribute="${CSS.escape(attribute)}"]`);
  if (scalar) {
    if (scalar instanceof HTMLTextAreaElement) setMarkdownControlValue(scalar, "");
    else scalar.value = "";
  }

  for (const setting of widget.settings) {
    if (
      setting.type === "table" &&
      (setting.attributes || []).includes(attribute)
    ) {
      resetTableSetting(form, setting, widget);
    }
  }
}

function activateExclusiveAttribute(
  form: HTMLFormElement,
  widget: CatalogWidget,
  attribute: string,
): void {
  if (!attribute) return;
  for (const constraint of widget.constraints || []) {
    if (
      constraint.kind !== "exactly-one" ||
      !constraint.attributes.includes(attribute)
    ) {
      continue;
    }
    for (const peer of constraint.attributes) {
      if (peer !== attribute) clearFormAttribute(form, widget, peer);
    }
  }
}

function collectFormValues(
  form: HTMLFormElement,
  widget: CatalogWidget,
  original: Record<string, string>,
): Record<string, string> {
  const result = { ...original };
  for (const control of form.querySelectorAll<
    HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  >("[data-widget-attribute]")) {
    const name = control.dataset.widgetAttribute || "";
    if (name) result[name] = control.value.trim();
  }
  for (const setting of widget.settings) {
    if (setting.type === "tree") {
      collectTreeValues(form, setting, widget, result);
      continue;
    }
    if (setting.type !== "table") continue;
    const rows = tableRows(form, setting);
    if (setting.row_separator && setting.attributes?.length === 1) {
      const name = setting.attributes[0];
      const attribute = widgetAttribute(widget, name);
      if (attribute) {
        result[name] = rows
          .map((row) => row.join(setting.row_separator))
          .join(attribute.separator || ";");
      }
      continue;
    }
    (setting.attributes || []).forEach((name, column) => {
      const attribute = widgetAttribute(widget, name);
      if (!attribute) return;
      const values = rows.map((row) => row[column] || "");
      const defaults = defaultTableColors(widget, values.length);
      if (
        attribute.type === "color-list" &&
        !(original[name] || "").trim() &&
        values.every(
          (value, index) =>
            normalizeWidgetColor(value, attribute) === defaults[index],
        )
      ) {
        result[name] = "";
        return;
      }
      result[name] = values.join(attribute.separator || ";");
    });
  }
  return result;
}

function positionPopover(popover: HTMLElement, anchor: HTMLElement): void {
  if (popover.classList.contains("visual-widget-popover-tree")) {
    popover.style.left = "50%";
    popover.style.top = "50%";
    popover.style.transform = "translate(-50%, -50%)";
    return;
  }
  popover.style.transform = "";
  const rect = anchor.getBoundingClientRect();
  popover.style.left = `${Math.max(
    8,
    Math.min(window.innerWidth - popover.offsetWidth - 8, rect.left),
  )}px`;
  const below = rect.bottom + 8;
  const above = rect.top - popover.offsetHeight - 8;
  popover.style.top = `${below + popover.offsetHeight < window.innerHeight ? below : Math.max(8, above)}px`;
}

export function createSettingsPopover(
  raw: string,
  widget: CatalogWidget,
  anchor: HTMLElement,
  apply: (raw: string) => void,
  preview: (raw: string) => void,
  close: (restorePreview: boolean) => void,
  completions: CatalogCompletion[],
  initialAnnotation?: { attribute: string; selection: string },
): HTMLElement {
  const popover = document.createElement("div");
  popover.className = "visual-widget-popover";
  if (widget.settings.some((setting) => setting.type === "tree"))
    popover.classList.add("visual-widget-popover-tree");
  popover.setAttribute("role", "dialog");
  popover.setAttribute("aria-label", `Edit ${widget.name}`);

  const heading = document.createElement("div");
  heading.className = "visual-widget-popover-title";
  heading.textContent = widget.name;
  popover.append(heading);

  const form = document.createElement("form");
  const values = widgetValues(raw, widget);
  const activate = (attribute: string) =>
    activateExclusiveAttribute(form, widget, attribute);
  for (const setting of widget.settings) {
    form.append(
      setting.type === "table"
        ? createTableSetting(setting, widget, values, activate)
        : setting.type === "tree"
          ? createTreeSetting(setting, widget, values)
          : createScalarSetting(setting, widget, values, completions),
    );
  }
  if (initialAnnotation) {
    const setting = widget.settings.find(
      (candidate) =>
        candidate.type === "table" &&
        candidate.row_separator &&
        candidate.attributes?.[0] === initialAnnotation.attribute,
    );
    const first = form.querySelector<HTMLInputElement>(
      `[data-widget-table-attribute="${CSS.escape(initialAnnotation.attribute)}"][data-widget-table-column="0"]`,
    );
    const body = first?.closest("table")?.tBodies[0];
    if (setting && body) {
      const empty = [...body.rows].find((row) =>
        [
          ...row.querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
            "input, textarea",
          ),
        ].every((control) => !control.value),
      );
      const row = empty || appendTableRow(body, setting, widget, []);
      const selection = row.querySelector<HTMLInputElement>(
        '[data-widget-table-column="0"]',
      );
      const text = row.querySelector<HTMLTextAreaElement | HTMLInputElement>(
        '[data-widget-table-column="1"]',
      );
      if (selection) selection.value = initialAnnotation.selection;
      requestAnimationFrame(() => text?.focus());
    }
  }

  const error = document.createElement("div");
  error.className = "visual-widget-form-error";
  error.hidden = true;
  error.setAttribute("role", "alert");
  form.append(error);

  const actions = document.createElement("div");
  actions.className = "visual-widget-popover-actions";
  const sourceButton = document.createElement("button");
  sourceButton.type = "button";
  sourceButton.textContent = "Edit source";
  const cancel = document.createElement("button");
  cancel.type = "button";
  cancel.textContent = "Cancel";
  const submit = document.createElement("button");
  submit.type = "submit";
  submit.className = "primary";
  submit.textContent = "Apply";
  actions.append(sourceButton, cancel, submit);
  form.append(actions);
  popover.append(form);

  // Opening a rich editor or changing tabs must not normalize untouched source.
  const sourceForValues = (next: Record<string, string>): string =>
    Object.entries(next).every(([name, value]) => value === (values[name] || ""))
      ? raw
      : rewriteWidgetSource(raw, widget, next);

  const refreshPreview = () => {
    const next = collectFormValues(form, widget, values);
    preview(sourceForValues(next));
  };

  form.addEventListener("input", (event) => {
    const control = event.target;
    if (
      !(control instanceof HTMLInputElement) &&
      !(control instanceof HTMLSelectElement) &&
      !(control instanceof HTMLTextAreaElement)
    )
      return;
    if (control.value.trim()) {
      activate(
        control.dataset.widgetExclusiveAttribute ||
          control.dataset.widgetAttribute ||
          "",
      );
    }
    error.hidden = true;
    refreshPreview();
  });

  sourceButton.addEventListener("click", () => {
    void openSourceDialog({
      title: `Edit ${widget.name} source`,
      source: raw,
      validate(source) {
        const parsed = matchWidgetSource(source, [widget], widget.inline);
        return parsed?.raw === source
          ? ""
          : `Source is not valid ${widget.name} syntax.`;
      },
    }).then((source) => {
      if (source === null) return;
      apply(source);
      close(false);
    });
  });
  cancel.addEventListener("click", () => close(true));
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    const treeProblem = treeFormProblem(form, widget);
    if (treeProblem) {
      error.textContent = treeProblem;
      error.hidden = false;
      return;
    }
    const tableProblem = compoundTableProblem(form, widget);
    if (tableProblem) {
      error.textContent = tableProblem;
      error.hidden = false;
      return;
    }
    const next = collectFormValues(form, widget, values);
    const problems = validateWidgetValues(next, widget);
    if (problems.length) {
      error.textContent = problems[0];
      error.hidden = false;
      return;
    }
    apply(sourceForValues(next));
    close(false);
  });

  document.body.append(popover);
  positionPopover(popover, anchor);
  requestAnimationFrame(() => positionPopover(popover, anchor));
  return popover;
}
