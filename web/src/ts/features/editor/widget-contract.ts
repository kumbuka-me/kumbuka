// Shared declarative visual-editor widget contract and Markdown mapping helpers.

import { isRecord, isStringRecord } from "../../core/guards.ts";

export type WidgetAttributeType =
  "string" | "identifier" | "enum" | "list" | "color-list";

export interface CatalogWidgetAttribute {
  name: string;
  type: WidgetAttributeType;
  default?: string;
  required?: boolean;
  max_bytes?: number;
  max_items?: number;
  values?: string[];
  separator?: string;
  fallback_separator?: string;
  unique?: boolean;
  aliases?: Record<string, string>;
}

export interface CatalogWidgetSettingColumn {
  label: string;
  type: "text" | "color";
}

export interface CatalogWidgetSetting {
  type: "text" | "select" | "table";
  label: string;
  attribute?: string;
  attributes?: string[];
  columns?: CatalogWidgetSettingColumn[];
  placeholder?: string;
  suggestions?: string[];
}

export interface CatalogWidgetConstraint {
  kind: "exactly-one" | "same-length" | "member-of";
  attributes: string[];
  optional?: boolean;
}

export interface CatalogWidgetBadgePreview {
  class: string;
  solid_class?: string;
  outline_class?: string;
  prefix_class?: string;
  value_class?: string;
  prefix_attribute?: string;
  label_attribute?: string;
  labels_attribute?: string;
  fallback_attribute?: string;
  colors_attribute?: string;
  style_attribute?: string;
  default_label?: string;
  default_colors?: string[];
  tone_classes?: Record<string, string>;
}

export interface CatalogWidget {
  plugin_id: string;
  id: string;
  name: string;
  inline: boolean;
  syntax: { kind: "macro"; name: string; multiline?: boolean };
  attributes: CatalogWidgetAttribute[];
  settings: CatalogWidgetSetting[];
  constraints?: CatalogWidgetConstraint[];
  preview: { kind: "badge"; badge: CatalogWidgetBadgePreview };
}

export interface CatalogWidgetProblem {
  plugin_id: string;
  message: string;
}

export interface ParsedMacroAttribute {
  name: string;
  value: string;
}

export interface ParsedMacro {
  name: string;
  raw: string;
  attributes: ParsedMacroAttribute[];
}

const identifier = /^[a-z0-9][a-z0-9._-]{0,127}$/;
const attributeName = /^[A-Za-z][A-Za-z0-9._-]{0,127}$/;
const widgetIdentifier = /^[A-Za-z0-9][A-Za-z0-9._:/-]*$/;
const color = /^#[0-9a-fA-F]{6}$/;

function strings(value: unknown): value is string[] {
  return (
    Array.isArray(value) && value.every((item) => typeof item === "string")
  );
}

function optionalString(value: unknown): boolean {
  return value === undefined || typeof value === "string";
}

function optionalBoolean(value: unknown): boolean {
  return value === undefined || typeof value === "boolean";
}

function optionalNumber(value: unknown): boolean {
  return value === undefined || (Number.isInteger(value) && Number(value) >= 0);
}

function isWidgetAttribute(value: unknown): value is CatalogWidgetAttribute {
  if (!isRecord(value)) return false;
  return (
    typeof value.name === "string" &&
    attributeName.test(value.name) &&
    ["string", "identifier", "enum", "list", "color-list"].includes(
      String(value.type),
    ) &&
    optionalString(value.default) &&
    optionalBoolean(value.required) &&
    optionalNumber(value.max_bytes) &&
    optionalNumber(value.max_items) &&
    (value.values === undefined || strings(value.values)) &&
    optionalString(value.separator) &&
    optionalString(value.fallback_separator) &&
    optionalBoolean(value.unique) &&
    (value.aliases === undefined || isStringRecord(value.aliases))
  );
}

function isWidgetColumn(value: unknown): value is CatalogWidgetSettingColumn {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    (value.type === "text" || value.type === "color")
  );
}

function isWidgetSetting(value: unknown): value is CatalogWidgetSetting {
  return (
    isRecord(value) &&
    (value.type === "text" ||
      value.type === "select" ||
      value.type === "table") &&
    typeof value.label === "string" &&
    optionalString(value.attribute) &&
    (value.attributes === undefined || strings(value.attributes)) &&
    (value.columns === undefined ||
      (Array.isArray(value.columns) && value.columns.every(isWidgetColumn))) &&
    optionalString(value.placeholder) &&
    (value.suggestions === undefined || strings(value.suggestions))
  );
}

function isWidgetConstraint(value: unknown): value is CatalogWidgetConstraint {
  return (
    isRecord(value) &&
    (value.kind === "exactly-one" ||
      value.kind === "same-length" ||
      value.kind === "member-of") &&
    strings(value.attributes) &&
    optionalBoolean(value.optional)
  );
}

function isBadgePreview(value: unknown): value is CatalogWidgetBadgePreview {
  if (!isRecord(value)) return false;
  return (
    typeof value.class === "string" &&
    optionalString(value.solid_class) &&
    optionalString(value.outline_class) &&
    optionalString(value.prefix_class) &&
    optionalString(value.value_class) &&
    optionalString(value.prefix_attribute) &&
    optionalString(value.label_attribute) &&
    optionalString(value.labels_attribute) &&
    optionalString(value.fallback_attribute) &&
    optionalString(value.colors_attribute) &&
    optionalString(value.style_attribute) &&
    optionalString(value.default_label) &&
    (value.default_colors === undefined || strings(value.default_colors)) &&
    (value.tone_classes === undefined || isStringRecord(value.tone_classes))
  );
}

export function isCatalogWidget(value: unknown): value is CatalogWidget {
  if (!isRecord(value) || !isRecord(value.syntax) || !isRecord(value.preview))
    return false;
  return (
    typeof value.plugin_id === "string" &&
    identifier.test(value.plugin_id) &&
    typeof value.id === "string" &&
    identifier.test(value.id) &&
    typeof value.name === "string" &&
    typeof value.inline === "boolean" &&
    value.syntax.kind === "macro" &&
    typeof value.syntax.name === "string" &&
    identifier.test(value.syntax.name) &&
    optionalBoolean(value.syntax.multiline) &&
    Array.isArray(value.attributes) &&
    value.attributes.every(isWidgetAttribute) &&
    Array.isArray(value.settings) &&
    value.settings.every(isWidgetSetting) &&
    (value.constraints === undefined ||
      (Array.isArray(value.constraints) &&
        value.constraints.every(isWidgetConstraint))) &&
    value.preview.kind === "badge" &&
    isBadgePreview(value.preview.badge)
  );
}

export function isCatalogWidgetProblem(
  value: unknown,
): value is CatalogWidgetProblem {
  return (
    isRecord(value) &&
    typeof value.plugin_id === "string" &&
    typeof value.message === "string"
  );
}

function decodeQuoted(value: string): string | null {
  try {
    return JSON.parse(value) as string;
  } catch {
    return null;
  }
}

function macroEnd(source: string): number {
  let quoted = false;
  let escaped = false;
  for (let index = 2; index < source.length - 1; index += 1) {
    const char = source[index];
    if (escaped) {
      escaped = false;
      continue;
    }
    if (quoted && char === "\\") {
      escaped = true;
      continue;
    }
    if (char === '"') {
      quoted = !quoted;
      continue;
    }
    if (!quoted && char === "}" && source[index + 1] === "}") return index + 2;
  }
  return -1;
}

// parseMacro reads one complete {{name key="value"}} token without consuming surrounding source.
export function parseMacro(source: string): ParsedMacro | null {
  if (!source.startsWith("{{")) return null;
  const end = macroEnd(source);
  if (end < 0) return null;
  const raw = source.slice(0, end);
  const body = raw.slice(2, -2);
  const nameMatch = /^\s*([a-z0-9][a-z0-9._-]{0,127})(?=\s|$)/.exec(body);
  if (!nameMatch) return null;

  const attributes: ParsedMacroAttribute[] = [];
  let rest = body.slice(nameMatch[0].length);
  const seen = new Set<string>();
  while (rest.trim().length) {
    rest = rest.trimStart();
    const name = /^([A-Za-z][A-Za-z0-9._-]{0,127})\s*=\s*/.exec(rest);
    if (!name) return null;
    const key = name[1];
    rest = rest.slice(name[0].length);
    if (!rest.startsWith('"') || seen.has(key)) return null;

    let index = 1;
    let escaped = false;
    for (; index < rest.length; index += 1) {
      const char = rest[index];
      if (escaped) {
        escaped = false;
        continue;
      }
      if (char === "\\") {
        escaped = true;
        continue;
      }
      if (char === '"') break;
    }
    if (index >= rest.length) return null;
    const quoted = rest.slice(0, index + 1);
    const value = decodeQuoted(quoted);
    if (value === null) return null;
    attributes.push({ name: key, value });
    seen.add(key);
    rest = rest.slice(index + 1);
    if (rest && !/^\s/.test(rest)) return null;
  }

  return { name: nameMatch[1], raw, attributes };
}

export function widgetForMacro(
  macro: ParsedMacro,
  widgets: CatalogWidget[],
): CatalogWidget | null {
  return widgets.find((widget) => widget.syntax.name === macro.name) ?? null;
}

// matchWidgetMacro matches one widget-backed macro at the start of source.
export function matchWidgetMacro(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): { raw: string; widget: CatalogWidget } | null {
  const macro = parseMacro(source);
  if (!macro) return null;
  const widget = widgetForMacro(macro, widgets);
  if (!widget || widget.inline !== inline) return null;
  if (!widget.syntax.multiline && macro.raw.includes("\n")) return null;
  if (inline && macro.raw.includes("\n")) return null;
  if (!inline && !/^[ \t]*(?:\r?\n|$)/.test(source.slice(macro.raw.length)))
    return null;
  return { raw: macro.raw, widget };
}

export function widgetAttribute(
  widget: CatalogWidget,
  name: string,
): CatalogWidgetAttribute | undefined {
  return widget.attributes.find((attribute) => attribute.name === name);
}

export function widgetValues(
  raw: string,
  widget: CatalogWidget,
): Record<string, string> {
  const macro = parseMacro(raw);
  const result: Record<string, string> = Object.create(null);
  for (const attribute of widget.attributes) {
    if (attribute.default !== undefined)
      result[attribute.name] = attribute.default;
  }
  if (!macro || macro.name !== widget.syntax.name) return result;
  for (const attribute of macro.attributes)
    result[attribute.name] = attribute.value;
  return result;
}

export function splitWidgetList(
  value: string,
  attribute: CatalogWidgetAttribute,
): string[] {
  if (!value.trim()) return [];
  let separator = attribute.separator || ";";
  if (
    attribute.fallback_separator &&
    !value.includes(separator) &&
    value.includes(attribute.fallback_separator)
  ) {
    separator = attribute.fallback_separator;
  }
  return value.split(separator).map((item) => item.trim());
}

export function normalizeWidgetColor(
  value: string,
  attribute?: CatalogWidgetAttribute,
): string | null {
  const normalized = value.trim().toLowerCase();
  const alias = attribute?.aliases?.[normalized];
  if (alias && color.test(alias)) return alias.toLowerCase();
  return color.test(normalized) ? normalized : null;
}

function encodedLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

// validateWidgetValues applies contract validation before a NodeView transaction changes source.
export function validateWidgetValues(
  values: Record<string, string>,
  widget: CatalogWidget,
): string[] {
  const errors: string[] = [];
  for (const attribute of widget.attributes) {
    const value = values[attribute.name] ?? attribute.default ?? "";
    if (attribute.required && !value.trim()) {
      errors.push(`${attribute.name} is required.`);
      continue;
    }
    if (!value) continue;

    if (attribute.type === "identifier" && !widgetIdentifier.test(value))
      errors.push(`${attribute.name} contains unsupported characters.`);
    if (attribute.type === "enum" && !(attribute.values || []).includes(value))
      errors.push(`${attribute.name} has an unsupported value.`);

    if (attribute.type === "list" || attribute.type === "color-list") {
      const items = splitWidgetList(value, attribute);
      if (attribute.max_items && items.length > attribute.max_items)
        errors.push(`${attribute.name} has too many items.`);
      for (const item of items) {
        if (!item) errors.push(`${attribute.name} contains an empty item.`);
        if (attribute.max_bytes && encodedLength(item) > attribute.max_bytes)
          errors.push(`${attribute.name} contains an item that is too long.`);
        if (
          attribute.type === "color-list" &&
          !normalizeWidgetColor(item, attribute)
        )
          errors.push(`${attribute.name} contains an invalid color.`);
      }
      if (attribute.unique && new Set(items).size !== items.length)
        errors.push(`${attribute.name} contains a duplicate item.`);
      continue;
    }

    if (attribute.max_bytes && encodedLength(value) > attribute.max_bytes)
      errors.push(`${attribute.name} is too long.`);
  }

  for (const constraint of widget.constraints || []) {
    const present = constraint.attributes.filter((name) =>
      Boolean((values[name] || "").trim()),
    );
    if (constraint.kind === "exactly-one" && present.length !== 1)
      errors.push(`Use exactly one of ${constraint.attributes.join(" or ")}.`);
    if (constraint.kind === "same-length") {
      const lengths = constraint.attributes.map((name) => {
        const attribute = widgetAttribute(widget, name);
        const value = values[name] || "";
        return attribute && value
          ? splitWidgetList(value, attribute).length
          : 0;
      });
      if (constraint.optional && lengths[lengths.length - 1] === 0) continue;
      if (new Set(lengths).size > 1)
        errors.push(
          `${constraint.attributes.join(" and ")} must have the same number of items.`,
        );
    }
    if (constraint.kind === "member-of") {
      const [valueName, listName] = constraint.attributes;
      const value = values[valueName] || "";
      const listAttribute = widgetAttribute(widget, listName);
      const listValue = values[listName] || "";
      if (!value) continue;
      if (!listValue && constraint.optional) continue;
      const allowed =
        listAttribute && listValue
          ? splitWidgetList(listValue, listAttribute)
          : [];
      if (!allowed.includes(value))
        errors.push(
          `${valueName} must match one of the configured ${listName}.`,
        );
    }
  }
  return [...new Set(errors)];
}

function quoteAttribute(value: string): string {
  return JSON.stringify(value);
}

// rewriteWidgetMacro updates declared attributes while preserving unknown attributes and their order.
export function rewriteWidgetMacro(
  raw: string,
  widget: CatalogWidget,
  values: Record<string, string>,
): string {
  const macro = parseMacro(raw);
  if (!macro || macro.name !== widget.syntax.name) return raw;
  const known = new Set(widget.attributes.map((attribute) => attribute.name));
  const emitted = new Set<string>();
  const parts: string[] = [];

  for (const attribute of macro.attributes) {
    if (!known.has(attribute.name)) {
      parts.push(`${attribute.name}=${quoteAttribute(attribute.value)}`);
      continue;
    }
    emitted.add(attribute.name);
    const value = values[attribute.name] ?? "";
    const declaration = widgetAttribute(widget, attribute.name);
    if (value === "" && !declaration?.required) continue;
    parts.push(`${attribute.name}=${quoteAttribute(value)}`);
  }

  for (const declaration of widget.attributes) {
    if (emitted.has(declaration.name)) continue;
    const value = values[declaration.name] ?? "";
    if (value === "" && !declaration.required) continue;
    if (
      !declaration.required &&
      declaration.default !== undefined &&
      value === declaration.default
    )
      continue;
    parts.push(`${declaration.name}=${quoteAttribute(value)}`);
  }

  return `{{${macro.name}${parts.length ? " " + parts.join(" ") : ""}}}`;
}
