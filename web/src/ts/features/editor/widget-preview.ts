// Host-rendered previews for declarative plugin widgets. No editor transactions.
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

export function renderWidgetBadge(
  root: HTMLElement,
  raw: string,
  widget: CatalogWidget,
): void {
  const preview = widget.preview.badge;
  const values = widgetValues(raw, widget);
  const state = badgeState(values, widget, preview);
  const classes = ["visual-widget-badge", preview.class];
  if (state.style === "outline" && preview.outline_class)
    classes.push(preview.outline_class);
  else if (preview.solid_class) classes.push(preview.solid_class);
  const tone = preview.tone_classes?.[state.color.toLowerCase()];
  if (tone) classes.push(tone);

  root.className = classes.join(" ");
  root.removeAttribute("style");
  if (state.style === "outline") {
    root.style.borderColor = state.color;
    root.style.color = state.color;
    root.style.backgroundColor = "transparent";
  } else {
    root.style.backgroundColor = state.color;
    root.style.color = readableForeground(state.color);
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
  root.replaceChildren(content);
  root.title = raw;
}
