// Declarative previews for plugin-provided visual-editor widgets.

import {
  normalizeWidgetColor,
  splitWidgetList,
  widgetAttribute,
  widgetValues,
  type CatalogWidget,
  type CatalogWidgetBadgePreview,
} from "./widget-contract.ts";

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

export function resetPreview(root: HTMLElement): void {
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

  const body = document.createElement("div");
  if (preview.body_class) body.className = preview.body_class;
  body.textContent = valueFor(values, preview.body_attribute);
  root.append(body);
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

export function renderWidget(
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
