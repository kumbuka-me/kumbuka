// Generated widget settings UI; the caller owns persistence and editor selection.
import {
  parseMacro,
  normalizeWidgetColor,
  rewriteWidgetMacro,
  splitWidgetList,
  validateWidgetValues,
  widgetAttribute,
  widgetValues,
  type CatalogWidget,
  type CatalogWidgetSetting,
} from "./widget-contract.ts";

function createLabel(text: string): HTMLLabelElement {
  const label = document.createElement("label");
  label.className = "visual-widget-field";
  const title = document.createElement("span");
  title.className = "visual-widget-field-label";
  title.textContent = text;
  label.append(title);
  return label;
}

function createScalarSetting(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): HTMLElement {
  const wrapper = createLabel(setting.label);
  const attribute = widgetAttribute(widget, setting.attribute || "");
  if (!attribute) return wrapper;

  let control: HTMLInputElement | HTMLSelectElement;
  if (setting.type === "select") {
    const select = document.createElement("select");
    for (const value of attribute.values || []) {
      const option = document.createElement("option");
      option.value = value;
      option.textContent = value;
      select.append(option);
    }
    control = select;
  } else {
    const input = document.createElement("input");
    input.type = "text";
    input.placeholder = setting.placeholder || "";
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

function listRows(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): string[][] {
  const lists = (setting.attributes || []).map((name) => {
    const attribute = widgetAttribute(widget, name);
    return attribute ? splitWidgetList(values[name] || "", attribute) : [];
  });
  const count = Math.max(1, ...lists.map((list) => list.length));
  const defaults = widget.preview.badge.default_colors || [];
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
    const input = document.createElement("input");
    const attributeName = setting.attributes?.[index] || "";
    const attribute = widgetAttribute(widget, attributeName);
    input.type = column.type;
    input.value =
      column.type === "color"
        ? normalizeWidgetColor(values[index] || "", attribute) || "#64748b"
        : values[index] || "";
    input.dataset.widgetTableAttribute = attributeName;
    if (activationAttribute)
      input.dataset.widgetExclusiveAttribute = activationAttribute;
    input.setAttribute("aria-label", column.label);
    cell.append(input);
  });
  const actions = row.insertCell();
  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "visual-widget-remove-row";
  remove.textContent = "Remove";
  remove.addEventListener("click", () => {
    const form = body.closest("form");
    if (body.rows.length > 1) row.remove();
    else
      row.querySelectorAll<HTMLInputElement>("input").forEach((input) => {
        input.value = input.type === "color" ? "#64748b" : "";
      });
    form?.dispatchEvent(new Event("input", { bubbles: true }));
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
    const colors = widget.preview.badge.default_colors || [];
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
    field.closest("form")?.dispatchEvent(new Event("input", { bubbles: true }));
    activate(exactlyOneActivationAttribute(widget, setting.attributes || []));
    row
      .querySelector<HTMLInputElement>('input[type="text"], input:not([type])')
      ?.focus();
  });
  field.append(add);
  return field;
}

function tableRows(
  form: HTMLFormElement,
  setting: CatalogWidgetSetting,
): string[][] {
  const attributes = setting.attributes || [];
  const table = form
    .querySelector<HTMLInputElement>(
      `[data-widget-table-attribute="${CSS.escape(attributes[0] || "")}"]`,
    )
    ?.closest("table");
  if (!table) return [];

  return [...(table.tBodies[0]?.rows || [])]
    .map((row) =>
      attributes.map((name) =>
        (
          row.querySelector<HTMLInputElement>(
            `[data-widget-table-attribute="${CSS.escape(name)}"]`,
          )?.value || ""
        ).trim(),
      ),
    )
    .filter((row) => Boolean(row[0]));
}

function defaultTableColors(widget: CatalogWidget, count: number): string[] {
  const colors = widget.preview.badge.default_colors || [];
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
  const input = form.querySelector<HTMLInputElement>(
    `[data-widget-table-attribute="${CSS.escape(name)}"]`,
  );
  const body = input?.closest("table")?.tBodies[0];
  if (!body) return;

  body.replaceChildren();
  appendTableRow(
    body,
    setting,
    widget,
    (setting.columns || []).map((column) =>
      column.type === "color"
        ? widget.preview.badge.default_colors?.[0] || "#64748b"
        : "",
    ),
  );
}

function clearFormAttribute(
  form: HTMLFormElement,
  widget: CatalogWidget,
  attribute: string,
): void {
  const scalar = form.querySelector<HTMLInputElement | HTMLSelectElement>(
    `[data-widget-attribute="${CSS.escape(attribute)}"]`,
  );
  if (scalar) scalar.value = "";

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
  const result: Record<string, string> = Object.assign(
    Object.create(null),
    original,
  );
  for (const control of form.querySelectorAll<
    HTMLInputElement | HTMLSelectElement
  >("[data-widget-attribute]")) {
    const name = control.dataset.widgetAttribute || "";
    if (name) result[name] = control.value.trim();
  }
  for (const setting of widget.settings) {
    if (setting.type !== "table") continue;
    const rows = tableRows(form, setting);
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
  const rect = anchor.getBoundingClientRect();
  popover.style.left = `${Math.max(
    8,
    Math.min(window.innerWidth - popover.offsetWidth - 8, rect.left),
  )}px`;
  const below = rect.bottom + 8;
  const above = rect.top - popover.offsetHeight - 8;
  popover.style.top = `${below + popover.offsetHeight < window.innerHeight ? below : Math.max(8, above)}px`;
}

export function createWidgetSettings(
  raw: string,
  widget: CatalogWidget,
  anchor: HTMLElement,
  apply: (raw: string) => void,
  close: () => void,
): HTMLElement {
  const popover = document.createElement("div");
  popover.className = "visual-widget-popover";
  popover.setAttribute("role", "dialog");
  popover.setAttribute("aria-label", `Edit ${widget.name}`);

  const heading = document.createElement("div");
  heading.className = "visual-widget-popover-title";
  heading.textContent = widget.name;
  popover.append(heading);

  const form = document.createElement("form");
  const values = widgetValues(raw, widget);
  let dirty = false;
  const activate = (attribute: string) =>
    activateExclusiveAttribute(form, widget, attribute);
  for (const setting of widget.settings) {
    form.append(
      setting.type === "table"
        ? createTableSetting(setting, widget, values, activate)
        : createScalarSetting(setting, widget, values),
    );
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

  form.addEventListener("input", (event) => {
    dirty = true;
    const control = event.target;
    if (!(
      control instanceof HTMLInputElement ||
      control instanceof HTMLSelectElement
    ))
      return;
    if (!control.value.trim()) return;
    activate(
      control.dataset.widgetExclusiveAttribute ||
        control.dataset.widgetAttribute ||
        "",
    );
    error.hidden = true;
  });

  sourceButton.addEventListener("click", () => {
    const source = window.prompt(`Edit ${widget.name} source`, raw);
    if (source === null) return;
    const parsed = parseMacro(source);
    if (
      !parsed ||
      parsed.raw !== source ||
      parsed.name !== widget.syntax.name ||
      (source.includes("\n") && (!widget.syntax.multiline || widget.inline))
    ) {
      error.textContent = `Source must remain a {{${widget.syntax.name} ...}} macro.`;
      error.hidden = false;
      return;
    }
    apply(source);
    close();
  });
  cancel.addEventListener("click", close);
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    if (!dirty) {
      close();
      return;
    }
    for (const setting of widget.settings) {
      if (setting.type !== "table") continue;
      const rows = tableRows(form, setting);
      for (const [column, name] of (setting.attributes || []).entries()) {
        const attribute = widgetAttribute(widget, name)!;
        if (
          rows.some(
            (row) =>
              row[column]?.includes(attribute.separator || ";") ||
              (rows.length === 1 &&
                attribute.fallback_separator &&
                row[column]?.includes(attribute.fallback_separator)),
          )
        ) {
          error.textContent = `${setting.label}: values cannot contain their list separators.`;
          error.hidden = false;
          return;
        }
      }
    }
    const next = collectFormValues(form, widget, values);
    const problems = validateWidgetValues(next, widget);
    if (problems.length) {
      error.textContent = problems[0];
      error.hidden = false;
      return;
    }
    apply(rewriteWidgetMacro(raw, widget, next));
    close();
  });

  document.body.append(popover);
  positionPopover(popover, anchor);
  requestAnimationFrame(() => {
    if (!popover.isConnected) return;
    positionPopover(popover, anchor);
    form
      .querySelector<HTMLInputElement | HTMLSelectElement>("input, select")
      ?.focus();
  });
  return popover;
}
