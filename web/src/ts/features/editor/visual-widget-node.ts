// Generic Tiptap NodeViews for plugin-provided declarative visual-editor widgets.

import { Node as TiptapNode, type AnyExtension } from "./visual-deps/core.ts";
import { openSourceDialog } from "./source-dialog.ts";
import {
  matchWidgetSource,
  normalizeWidgetColor,
  rewriteWidgetSource,
  splitWidgetList,
  validateWidgetValues,
  widgetAttribute,
  widgetForSource,
  widgetValues,
  type CatalogWidget,
  type CatalogWidgetBadgePreview,
  type CatalogWidgetSetting,
} from "./widget-contract.ts";

interface WidgetNodeViewContext {
  editor: any;
  getPos: () => number | undefined;
  node: any;
}

const sourceMarkers = ["{{", "!!! ", "???", '=== "'];

function firstWidgetSourceIndex(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): number {
  let offset = 0;

  while (offset < source.length) {
    let index = -1;
    for (const marker of sourceMarkers) {
      const candidate = source.indexOf(marker, offset);
      if (candidate >= 0 && (index < 0 || candidate < index)) index = candidate;
    }
    if (index < 0) return -1;
    if (
      (!inline && index > 0 && source[index - 1] !== "\n") ||
      !matchWidgetSource(source.slice(index), widgets, inline)
    ) {
      offset = index + 1;
      continue;
    }
    return index;
  }

  return -1;
}

function contractForRaw(
  raw: string,
  widgets: CatalogWidget[],
): CatalogWidget | null {
  return widgetForSource(raw, widgets);
}

function valueFor(values: Record<string, string>, name?: string): string {
  return name ? values[name] || "" : "";
}

function readableForeground(color: string): string {
  const red = Number.parseInt(color.slice(1, 3), 16);
  const green = Number.parseInt(color.slice(3, 5), 16);
  const blue = Number.parseInt(color.slice(5, 7), 16);
  return (red * 299 + green * 587 + blue * 114) / 1000 >= 150
    ? "#111827"
    : "#ffffff";
}

function badgeState(
  values: Record<string, string>,
  widget: CatalogWidget,
  preview: CatalogWidgetBadgePreview,
): { label: string; prefix: string; color: string; style: string } {
  const labelsAttribute = preview.labels_attribute
    ? widgetAttribute(widget, preview.labels_attribute)
    : undefined;
  const colorsAttribute = preview.colors_attribute
    ? widgetAttribute(widget, preview.colors_attribute)
    : undefined;
  const labels = labelsAttribute
    ? splitWidgetList(
        valueFor(values, preview.labels_attribute),
        labelsAttribute,
      )
    : [];
  const configuredColors = colorsAttribute
    ? splitWidgetList(
        valueFor(values, preview.colors_attribute),
        colorsAttribute,
      )
    : [];
  const requested = valueFor(values, preview.label_attribute);
  const index = requested ? labels.indexOf(requested) : -1;
  const selectedIndex = index >= 0 ? index : 0;
  const label =
    (requested && (index >= 0 || labels.length === 0)
      ? requested
      : labels[0]) ||
    valueFor(values, preview.fallback_attribute) ||
    preview.default_label ||
    widget.name;
  const rawColor =
    configuredColors[selectedIndex] ||
    preview.default_colors?.[
      selectedIndex % Math.max(1, preview.default_colors.length)
    ] ||
    "#64748b";
  const color = normalizeWidgetColor(rawColor, colorsAttribute) || "#64748b";
  const style = valueFor(values, preview.style_attribute) || "solid";
  return {
    label,
    prefix: valueFor(values, preview.prefix_attribute),
    color,
    style,
  };
}

function resetPreview(root: HTMLElement): void {
  root.className = "";
  root.removeAttribute("style");
  root.replaceChildren();
}

function renderBadge(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "badge") return;
  const preview = widget.preview.badge;
  const values = widgetValues(raw, widget);
  const state = badgeState(values, widget, preview);
  const classes = [preview.class];
  if (state.style === "outline" && preview.outline_class)
    classes.push(preview.outline_class);
  else if (preview.solid_class) classes.push(preview.solid_class);
  const tone = preview.tone_classes?.[state.color.toLowerCase()];
  if (tone) classes.push(tone);

  resetPreview(root);
  root.className = classes.join(" ");
  if (!tone) {
    if (state.style === "outline") {
      root.style.borderColor = state.color;
      root.style.color = state.color;
      root.style.backgroundColor = "transparent";
    } else {
      root.style.backgroundColor = state.color;
      root.style.color = readableForeground(state.color);
    }
  }

  const content = document.createDocumentFragment();
  if (state.prefix) {
    const prefix = document.createElement("span");
    if (preview.prefix_class) prefix.className = preview.prefix_class;
    prefix.textContent = state.prefix;
    content.append(prefix);
  }
  const value = document.createElement("span");
  if (preview.value_class) value.className = preview.value_class;
  value.textContent = state.label;
  content.append(value);
  root.append(content);
  root.title = raw;
}

function renderReference(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "reference") return;
  const preview = widget.preview.reference;
  const values = widgetValues(raw, widget);
  resetPreview(root);
  root.className = `${preview.class} visual-widget-reference`;

  const kind = document.createElement("span");
  kind.className = "visual-widget-reference-kind";
  kind.textContent = preview.prefix;
  const value = document.createElement("span");
  value.className = "visual-widget-reference-value";
  value.textContent =
    valueFor(values, preview.value_attribute) ||
    preview.default_value ||
    widget.name;
  root.append(kind, value);
  root.title = raw;
}

function renderCard(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "card") return;
  const preview = widget.preview.card;
  const values = widgetValues(raw, widget);
  resetPreview(root);
  root.className = `${preview.class} visual-widget-card`;

  const header = document.createElement("div");
  header.className = "visual-widget-card-header";
  const title = document.createElement("strong");
  title.className = preview.title_class || "visual-widget-card-title";
  title.textContent = preview.title;
  header.append(title);

  const subtitleValue = valueFor(values, preview.subtitle_attribute);
  if (subtitleValue) {
    const subtitle = document.createElement("span");
    subtitle.className =
      preview.subtitle_class || "visual-widget-card-subtitle";
    subtitle.textContent = subtitleValue;
    header.append(subtitle);
  }
  root.append(header);

  const metadataValues = (preview.metadata_attributes || [])
    .map((name) => valueFor(values, name))
    .filter(Boolean);
  if (metadataValues.length) {
    const metadata = document.createElement("div");
    metadata.className =
      preview.metadata_class || "visual-widget-card-metadata";
    metadata.textContent = metadataValues.join(" · ");
    root.append(metadata);
  }
  if (preview.body_text) {
    const body = document.createElement("div");
    body.className = "visual-widget-card-body";
    body.textContent = preview.body_text;
    root.append(body);
  }
  root.title = raw;
}

function renderCallout(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "callout") return;
  const preview = widget.preview.callout;
  const values = widgetValues(raw, widget);
  const kindValue = valueFor(values, preview.kind_attribute) || "note";
  resetPreview(root);
  root.className = `${preview.class} ${kindValue}`;

  const heading = document.createElement("strong");
  heading.textContent = kindValue
    ? kindValue[0].toUpperCase() + kindValue.slice(1)
    : widget.name;
  const body = document.createElement("div");
  if (preview.body_class) body.className = preview.body_class;
  body.textContent = valueFor(values, preview.body_attribute);
  root.append(heading, body);
  root.title = raw;
}

function renderDetails(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "details") return;
  const preview = widget.preview.details;
  const values = widgetValues(raw, widget);
  resetPreview(root);

  const details = document.createElement("details");
  details.className = preview.class;
  details.open = valueFor(values, preview.open_attribute) === "true";
  const summary = document.createElement("summary");
  summary.textContent = valueFor(values, preview.title_attribute) || "Details";
  const body = document.createElement("div");
  if (preview.body_class) body.className = preview.body_class;
  body.textContent = valueFor(values, preview.body_attribute);
  details.append(summary, body);
  root.append(details);
  root.title = raw;
}

function renderTabs(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  if (widget.preview.kind !== "tabs") return;
  const preview = widget.preview.tabs;
  const values = widgetValues(raw, widget);
  const titleAttribute = widgetAttribute(widget, preview.titles_attribute);
  const bodyAttribute = widgetAttribute(widget, preview.bodies_attribute);
  const titles = titleAttribute
    ? splitWidgetList(
        valueFor(values, preview.titles_attribute),
        titleAttribute,
      )
    : [];
  const bodies = bodyAttribute
    ? splitWidgetList(valueFor(values, preview.bodies_attribute), bodyAttribute)
    : [];
  resetPreview(root);
  root.className = preview.class;

  const list = document.createElement("div");
  list.className = preview.list_class;
  const panels = document.createElement("div");
  panels.className = preview.panels_class;
  titles.forEach((title, index) => {
    const tab = document.createElement("button");
    tab.type = "button";
    tab.tabIndex = -1;
    tab.className = [preview.tab_class, index === 0 ? preview.active_class : ""]
      .filter(Boolean)
      .join(" ");
    tab.textContent = title;
    const panel = document.createElement("div");
    panel.className = [
      preview.panel_class,
      index === 0 ? "" : preview.hidden_class,
    ]
      .filter(Boolean)
      .join(" ");
    panel.textContent = bodies[index] || "";
    list.append(tab);
    panels.append(panel);
  });
  root.append(list, panels);
  root.title = raw;
}

function renderWidget(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  switch (widget.preview.kind) {
    case "badge":
      renderBadge(root, raw, widget);
      break;
    case "reference":
      renderReference(root, raw, widget);
      break;
    case "card":
      renderCard(root, raw, widget);
      break;
    case "callout":
      renderCallout(root, raw, widget);
      break;
    case "details":
      renderDetails(root, raw, widget);
      break;
    case "tabs":
      renderTabs(root, raw, widget);
      break;
  }
}

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

  let control: HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;
  if (setting.type === "select") {
    const select = document.createElement("select");
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
    control = select;
  } else if (setting.type === "textarea") {
    const textarea = document.createElement("textarea");
    textarea.rows = 5;
    textarea.placeholder = setting.placeholder || "";
    control = textarea;
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
    const attributeName = setting.attributes?.[index] || "";
    const attribute = widgetAttribute(widget, attributeName);
    const control =
      column.type === "textarea"
        ? document.createElement("textarea")
        : document.createElement("input");
    if (control instanceof HTMLInputElement) control.type = column.type;
    if (control instanceof HTMLTextAreaElement) control.rows = 3;
    control.value =
      column.type === "color"
        ? normalizeWidgetColor(values[index] || "", attribute) || "#64748b"
        : values[index] || "";
    control.dataset.widgetTableAttribute = attributeName;
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
    if (body.rows.length > 1) row.remove();
    else
      row
        .querySelectorAll<HTMLInputElement | HTMLTextAreaElement>(
          "input, textarea",
        )
        .forEach((control) => {
          control.value =
            control instanceof HTMLInputElement && control.type === "color"
              ? "#64748b"
              : "";
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
    row
      .querySelector<HTMLInputElement | HTMLTextAreaElement>(
        'input[type="text"], input:not([type]), textarea',
      )
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
  const first = form.querySelector<HTMLInputElement | HTMLTextAreaElement>(
    `[data-widget-table-attribute="${CSS.escape(attributes[0] || "")}"]`,
  );
  const table = first?.closest("table");
  if (!table) return [];

  return [...(table.tBodies[0]?.rows || [])]
    .map((row) =>
      attributes.map((name) =>
        (
          row.querySelector<HTMLInputElement | HTMLTextAreaElement>(
            `[data-widget-table-attribute="${CSS.escape(name)}"]`,
          )?.value || ""
        ).trim(),
      ),
    )
    .filter((row) => Boolean(row[0]));
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
  const result = { ...original };
  for (const control of form.querySelectorAll<
    HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
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

function createSettingsPopover(
  raw: string,
  widget: CatalogWidget,
  anchor: HTMLElement,
  apply: (raw: string) => void,
  preview: (raw: string) => void,
  close: (restorePreview: boolean) => void,
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

  const refreshPreview = () => {
    const next = collectFormValues(form, widget, values);
    preview(rewriteWidgetSource(raw, widget, next));
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
    const next = collectFormValues(form, widget, values);
    const problems = validateWidgetValues(next, widget);
    if (problems.length) {
      error.textContent = problems[0];
      error.hidden = false;
      return;
    }
    apply(rewriteWidgetSource(raw, widget, next));
    close(false);
  });

  document.body.append(popover);
  positionPopover(popover, anchor);
  requestAnimationFrame(() => positionPopover(popover, anchor));
  return popover;
}

function widgetNodeView(
  widgets: CatalogWidget[],
  context: WidgetNodeViewContext,
  inline: boolean,
): any {
  let node = context.node;
  let popover: HTMLElement | null = null;
  const shell = document.createElement(inline ? "span" : "div");
  shell.className = inline
    ? "visual-widget-node"
    : "visual-widget-node visual-widget-node-block";
  shell.contentEditable = "false";
  shell.dataset.visualWidget = "";

  const preview = document.createElement(inline ? "span" : "div");
  shell.append(preview);

  const render = () => {
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) {
      preview.className = "visual-kumbuka-token";
      preview.textContent = "Widget";
      preview.title = raw;
      return;
    }
    renderWidget(preview, raw, widget);
    shell.dataset.pluginId = widget.plugin_id;
    shell.dataset.widgetId = widget.id;
  };
  const close = (restorePreview = true) => {
    popover?.remove();
    popover = null;
    if (restorePreview) render();
  };
  const apply = (raw: string) => {
    const position = context.getPos();
    if (typeof position !== "number") return;
    const transaction = context.editor.state.tr.setNodeMarkup(
      position,
      undefined,
      {
        ...node.attrs,
        raw,
      },
    );
    context.editor.view.dispatch(transaction);
  };
  const open = () => {
    const raw = String(node.attrs?.raw || "");
    const widget = contractForRaw(raw, widgets);
    if (!widget) return;
    close();
    popover = createSettingsPopover(
      raw,
      widget,
      shell,
      apply,
      (previewRaw) => renderWidget(preview, previewRaw, widget),
      close,
    );
  };

  shell.addEventListener("click", (event) => {
    event.preventDefault();
    const position = context.getPos();
    if (typeof position === "number")
      context.editor.chain().focus().setNodeSelection(position).run();
    open();
  });
  render();

  return {
    dom: shell,
    selectNode() {
      shell.classList.add("ProseMirror-selectednode");
    },
    deselectNode() {
      shell.classList.remove("ProseMirror-selectednode");
      close();
    },
    update(updated: any) {
      if (updated.type !== node.type) return false;
      node = updated;
      close(false);
      render();
      return true;
    },
    stopEvent(event: Event) {
      return shell.contains(event.target as Node);
    },
    ignoreMutation() {
      return true;
    },
    destroy() {
      close(false);
    },
  };
}

function widgetNode(widgets: CatalogWidget[], inline: boolean): AnyExtension {
  const nodeName = inline ? "kumbukaWidgetInline" : "kumbukaWidgetBlock";
  const tokenName = inline ? "kumbuka_widget_inline" : "kumbuka_widget_block";
  return TiptapNode.create({
    name: nodeName,
    priority: 200,
    inline,
    group: inline ? "inline" : "block",
    atom: true,
    selectable: true,
    draggable: !inline,
    addAttributes() {
      return {
        raw: {
          default: "",
          parseHTML: (element: HTMLElement) =>
            element.getAttribute("data-kumbuka-raw") || "",
        },
      };
    },
    parseHTML() {
      return [
        {
          tag: inline ? "span[data-visual-widget]" : "div[data-visual-widget]",
        },
      ];
    },
    renderHTML({ node }: any) {
      return [
        inline ? "span" : "div",
        {
          "data-visual-widget": "",
          "data-kumbuka-raw": String(node.attrs?.raw || ""),
        },
      ];
    },
    addNodeView() {
      return (context: WidgetNodeViewContext) =>
        widgetNodeView(widgets, context, inline);
    },
    markdownTokenName: tokenName,
    markdownTokenizer: {
      name: tokenName,
      level: inline ? "inline" : "block",
      start(source: string) {
        return firstWidgetSourceIndex(source, widgets, inline);
      },
      tokenize(source: string) {
        const matched = matchWidgetSource(source, widgets, inline);
        if (!matched) return undefined;
        const consumed =
          !inline && source.startsWith(matched.raw + "\n")
            ? matched.raw + "\n"
            : matched.raw;
        return { type: tokenName, raw: consumed, text: matched.raw };
      },
    },
    parseMarkdown(token: any) {
      return {
        type: nodeName,
        attrs: {
          raw: String(token.text || token.raw || "").replace(/\n$/, ""),
        },
      };
    },
    renderMarkdown(node: any) {
      const raw = String(node.attrs?.raw || "");
      return inline ? raw : raw ? raw + "\n\n" : "";
    },
  });
}

// visualWidgetNodes creates the inline and block NodeViews backed by active plugin contracts.
export function visualWidgetNodes(widgets: CatalogWidget[]): AnyExtension[] {
  const extensions: AnyExtension[] = [];
  if (widgets.some((widget) => widget.inline))
    extensions.push(widgetNode(widgets, true));
  if (widgets.some((widget) => !widget.inline))
    extensions.push(widgetNode(widgets, false));
  return extensions;
}
