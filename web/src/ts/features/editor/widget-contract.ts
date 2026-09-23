// Shared declarative visual-editor widget contract and Markdown mapping helpers.

import { isRecord, isStringRecord } from "../../core/guards.ts";

export type WidgetAttributeType =
  "string" | "identifier" | "enum" | "list" | "color-list";

export type WidgetSyntaxKind =
  "macro" | "substitution" | "callout" | "details" | "tabs";

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
  repeat?: boolean;
  emit_empty?: boolean;
  aliases?: Record<string, string>;
}

export interface CatalogWidgetSettingColumn {
  label: string;
  type: "text" | "textarea" | "color";
}

export interface CatalogWidgetSetting {
  type: "text" | "textarea" | "select" | "resource" | "table";
  label: string;
  attribute?: string;
  attributes?: string[];
  columns?: CatalogWidgetSettingColumn[];
  placeholder?: string;
  suggestions?: string[];
  completion_module_id?: string;
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

export interface CatalogWidgetReferencePreview {
  class: string;
  prefix: string;
  value_attribute: string;
  default_value?: string;
}

export interface CatalogWidgetCardPreview {
  class: string;
  title: string;
  title_class?: string;
  subtitle_attribute?: string;
  subtitle_class?: string;
  metadata_attributes?: string[];
  metadata_class?: string;
  body_text?: string;
}

export interface CatalogWidgetCalloutPreview {
  class: string;
  body_class?: string;
  kind_attribute: string;
  body_attribute: string;
}

export interface CatalogWidgetDetailsPreview {
  class: string;
  body_class?: string;
  title_attribute: string;
  open_attribute: string;
  body_attribute: string;
}

export interface CatalogWidgetTabsPreview {
  class: string;
  list_class: string;
  tab_class: string;
  active_class?: string;
  panels_class: string;
  panel_class: string;
  hidden_class?: string;
  titles_attribute: string;
  bodies_attribute: string;
}

export type CatalogWidgetPreview =
  | { kind: "badge"; badge: CatalogWidgetBadgePreview }
  | { kind: "reference"; reference: CatalogWidgetReferencePreview }
  | { kind: "card"; card: CatalogWidgetCardPreview }
  | { kind: "callout"; callout: CatalogWidgetCalloutPreview }
  | { kind: "details"; details: CatalogWidgetDetailsPreview }
  | { kind: "tabs"; tabs: CatalogWidgetTabsPreview };

export interface CatalogWidget {
  plugin_id: string;
  id: string;
  name: string;
  inline: boolean;
  syntax: { kind: WidgetSyntaxKind; name?: string; multiline?: boolean };
  attributes: CatalogWidgetAttribute[];
  settings: CatalogWidgetSetting[];
  constraints?: CatalogWidgetConstraint[];
  preview: CatalogWidgetPreview;
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

export interface MatchedWidgetSource {
  raw: string;
  widget: CatalogWidget;
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
    optionalBoolean(value.repeat) &&
    optionalBoolean(value.emit_empty) &&
    (value.aliases === undefined || isStringRecord(value.aliases))
  );
}

function isWidgetColumn(value: unknown): value is CatalogWidgetSettingColumn {
  return (
    isRecord(value) &&
    typeof value.label === "string" &&
    (value.type === "text" ||
      value.type === "textarea" ||
      value.type === "color")
  );
}

function isWidgetSetting(value: unknown): value is CatalogWidgetSetting {
  const resourceSetting =
    isRecord(value) &&
    (value.type === "resource"
      ? typeof value.completion_module_id === "string" &&
        identifier.test(value.completion_module_id)
      : value.completion_module_id === undefined);
  return (
    isRecord(value) &&
    resourceSetting &&
    (value.type === "text" ||
      value.type === "textarea" ||
      value.type === "select" ||
      value.type === "resource" ||
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

function isReferencePreview(
  value: unknown,
): value is CatalogWidgetReferencePreview {
  return (
    isRecord(value) &&
    typeof value.class === "string" &&
    typeof value.prefix === "string" &&
    typeof value.value_attribute === "string" &&
    optionalString(value.default_value)
  );
}

function isCardPreview(value: unknown): value is CatalogWidgetCardPreview {
  return (
    isRecord(value) &&
    typeof value.class === "string" &&
    typeof value.title === "string" &&
    optionalString(value.title_class) &&
    optionalString(value.subtitle_attribute) &&
    optionalString(value.subtitle_class) &&
    (value.metadata_attributes === undefined ||
      strings(value.metadata_attributes)) &&
    optionalString(value.metadata_class) &&
    optionalString(value.body_text)
  );
}

function isCalloutPreview(
  value: unknown,
): value is CatalogWidgetCalloutPreview {
  return (
    isRecord(value) &&
    typeof value.class === "string" &&
    optionalString(value.body_class) &&
    typeof value.kind_attribute === "string" &&
    typeof value.body_attribute === "string"
  );
}

function isDetailsPreview(
  value: unknown,
): value is CatalogWidgetDetailsPreview {
  return (
    isRecord(value) &&
    typeof value.class === "string" &&
    optionalString(value.body_class) &&
    typeof value.title_attribute === "string" &&
    typeof value.open_attribute === "string" &&
    typeof value.body_attribute === "string"
  );
}

function isTabsPreview(value: unknown): value is CatalogWidgetTabsPreview {
  return (
    isRecord(value) &&
    typeof value.class === "string" &&
    typeof value.list_class === "string" &&
    typeof value.tab_class === "string" &&
    optionalString(value.active_class) &&
    typeof value.panels_class === "string" &&
    typeof value.panel_class === "string" &&
    optionalString(value.hidden_class) &&
    typeof value.titles_attribute === "string" &&
    typeof value.bodies_attribute === "string"
  );
}

function isWidgetPreview(value: unknown): value is CatalogWidgetPreview {
  if (!isRecord(value)) return false;
  switch (value.kind) {
    case "badge":
      return isBadgePreview(value.badge);
    case "reference":
      return isReferencePreview(value.reference);
    case "card":
      return isCardPreview(value.card);
    case "callout":
      return isCalloutPreview(value.callout);
    case "details":
      return isDetailsPreview(value.details);
    case "tabs":
      return isTabsPreview(value.tabs);
    default:
      return false;
  }
}

export function isCatalogWidget(value: unknown): value is CatalogWidget {
  if (!isRecord(value) || !isRecord(value.syntax)) return false;
  const kind = value.syntax.kind;
  const named = kind === "macro" || kind === "substitution";
  return (
    typeof value.plugin_id === "string" &&
    identifier.test(value.plugin_id) &&
    typeof value.id === "string" &&
    identifier.test(value.id) &&
    typeof value.name === "string" &&
    typeof value.inline === "boolean" &&
    (kind === "macro" ||
      kind === "substitution" ||
      kind === "callout" ||
      kind === "details" ||
      kind === "tabs") &&
    (named
      ? typeof value.syntax.name === "string" &&
        identifier.test(value.syntax.name)
      : value.syntax.name === undefined) &&
    optionalBoolean(value.syntax.multiline) &&
    Array.isArray(value.attributes) &&
    value.attributes.every(isWidgetAttribute) &&
    Array.isArray(value.settings) &&
    value.settings.every(isWidgetSetting) &&
    (value.constraints === undefined ||
      (Array.isArray(value.constraints) &&
        value.constraints.every(isWidgetConstraint))) &&
    isWidgetPreview(value.preview)
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
    const decoded = JSON.parse(value) as unknown;
    return typeof decoded === "string" ? decoded : null;
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
  while (rest.trim().length) {
    rest = rest.trimStart();
    const name = /^([A-Za-z][A-Za-z0-9._-]{0,127})\s*=\s*/.exec(rest);
    if (!name) return null;
    const key = name[1];
    rest = rest.slice(name[0].length);

    let value = "";
    if (rest.startsWith('"')) {
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
      const decoded = decodeQuoted(quoted);
      if (decoded === null) return null;
      value = decoded;
      rest = rest.slice(index + 1);
    } else {
      const bare = /^([^\s}]+)/.exec(rest);
      if (!bare) return null;
      value = bare[1];
      rest = rest.slice(bare[0].length);
    }
    attributes.push({ name: key, value });
  }

  return { name: nameMatch[1], raw, attributes };
}

export function widgetForMacro(
  macro: ParsedMacro,
  widgets: CatalogWidget[],
): CatalogWidget | null {
  return (
    widgets.find(
      (widget) =>
        widget.syntax.kind === "macro" && widget.syntax.name === macro.name,
    ) ?? null
  );
}

// matchWidgetMacro matches one widget-backed macro at the start of source.
export function matchWidgetMacro(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): MatchedWidgetSource | null {
  const macro = parseMacro(source);
  if (!macro) return null;
  const widget = widgetForMacro(macro, widgets);
  if (!widget || widget.inline !== inline) return null;
  if (!widget.syntax.multiline && macro.raw.includes("\n")) return null;
  if (inline && macro.raw.includes("\n")) return null;
  return { raw: macro.raw, widget };
}

function matchSubstitution(
  source: string,
  widget: CatalogWidget,
): string | null {
  const prefix = `{{${widget.syntax.name}:`;
  if (!source.startsWith(prefix)) return null;
  const end = source.indexOf("}}", prefix.length);
  if (end < 0) return null;
  const raw = source.slice(0, end + 2);
  if (raw.includes("\n")) return null;
  const value = raw.slice(prefix.length, -2).trim();
  return value ? raw : null;
}

function matchCallout(source: string): string | null {
  if (!source.startsWith("!!! ")) return null;
  const lines = source.split("\n");
  if (!/^!!!\s+[A-Za-z][A-Za-z0-9_-]*\s*$/.test(lines[0])) return null;
  let end = 1;
  while (end < lines.length && lines[end].trim() !== "") end += 1;
  return lines.slice(0, end).join("\n");
}

function parseQuotedTitle(line: string, marker: string): string | null {
  const rest = line.slice(marker.length).trim();
  if (!rest.startsWith('"')) return null;
  const title = decodeQuoted(rest);
  return title === null ? null : title;
}

function indentedBlockEnd(lines: string[], start: number): number {
  let end = start;
  while (end < lines.length) {
    const line = lines[end];
    if (line.trim() === "") {
      end += 1;
      continue;
    }
    if (line.startsWith("    ") || line.startsWith("\t")) {
      end += 1;
      continue;
    }
    break;
  }
  while (end > start && lines[end - 1].trim() === "") end -= 1;
  return end;
}

function matchDetails(source: string): string | null {
  const lines = source.split("\n");
  const marker = lines[0].startsWith("???+") ? "???+" : "???";
  if (
    !lines[0].startsWith(marker) ||
    parseQuotedTitle(lines[0], marker) === null
  )
    return null;
  const end = indentedBlockEnd(lines, 1);
  return lines.slice(0, end).join("\n");
}

function matchTabs(source: string): string | null {
  const lines = source.split("\n");
  if (
    !lines[0].startsWith('=== "') ||
    parseQuotedTitle(lines[0], "===") === null
  )
    return null;

  let index = 0;
  let end = 0;
  while (index < lines.length) {
    if (
      !lines[index].startsWith('=== "') ||
      parseQuotedTitle(lines[index], "===") === null
    )
      break;
    const bodyEnd = indentedBlockEnd(lines, index + 1);
    end = Math.max(index + 1, bodyEnd);
    index = bodyEnd;
    while (index < lines.length && lines[index].trim() === "") index += 1;
    if (index >= lines.length || !lines[index].startsWith('=== "')) break;
  }
  return lines.slice(0, end).join("\n");
}

function sourceForWidget(source: string, widget: CatalogWidget): string | null {
  switch (widget.syntax.kind) {
    case "macro": {
      const macro = parseMacro(source);
      if (!macro || macro.name !== widget.syntax.name) return null;
      if (!widget.syntax.multiline && macro.raw.includes("\n")) return null;
      return macro.raw;
    }
    case "substitution":
      return matchSubstitution(source, widget);
    case "callout":
      return matchCallout(source);
    case "details":
      return matchDetails(source);
    case "tabs":
      return matchTabs(source);
  }
}

// matchWidgetSource matches any contract-backed source at the start of source.
export function matchWidgetSource(
  source: string,
  widgets: CatalogWidget[],
  inline: boolean,
): MatchedWidgetSource | null {
  for (const widget of widgets) {
    if (widget.inline !== inline) continue;
    const raw = sourceForWidget(source, widget);
    if (raw) return { raw, widget };
  }
  return null;
}

// widgetForSource resolves one complete raw widget source to its owning contract.
export function widgetForSource(
  raw: string,
  widgets: CatalogWidget[],
): CatalogWidget | null {
  for (const widget of widgets) {
    const matched = sourceForWidget(raw, widget);
    if (matched === raw) return widget;
  }
  return null;
}

export function widgetAttribute(
  widget: CatalogWidget,
  name: string,
): CatalogWidgetAttribute | undefined {
  return widget.attributes.find((attribute) => attribute.name === name);
}

function firstWidgetAttribute(
  widget: CatalogWidget,
): CatalogWidgetAttribute | undefined {
  return widget.attributes[0];
}

function macroWidgetValues(
  raw: string,
  widget: CatalogWidget,
  result: Record<string, string>,
): void {
  const macro = parseMacro(raw);
  if (!macro || macro.name !== widget.syntax.name) return;
  for (const declaration of widget.attributes) {
    const matching = macro.attributes.filter(
      (attribute) => attribute.name === declaration.name,
    );
    if (!matching.length) continue;
    if (declaration.repeat) {
      result[declaration.name] = matching
        .map((attribute) => attribute.value)
        .join(declaration.separator || ";");
    } else {
      result[declaration.name] = matching[matching.length - 1].value;
    }
  }
}

function substitutionWidgetValues(
  raw: string,
  widget: CatalogWidget,
  result: Record<string, string>,
): void {
  const matched = matchSubstitution(raw, widget);
  const declaration = firstWidgetAttribute(widget);
  if (!matched || !declaration) return;
  const prefix = `{{${widget.syntax.name}:`;
  result[declaration.name] = matched.slice(prefix.length, -2).trim();
}

function calloutWidgetValues(
  raw: string,
  result: Record<string, string>,
): void {
  const lines = raw.split("\n");
  result.kind = lines[0].slice(4).trim();
  result.body = lines.slice(1).join("\n");
}

function deindentBody(lines: string[]): string {
  const body = [...lines];
  while (body.length && body[0].trim() === "") body.shift();
  while (body.length && body[body.length - 1].trim() === "") body.pop();
  return body
    .map((line) => {
      if (line.startsWith("    ")) return line.slice(4);
      if (line.startsWith("\t")) return line.slice(1);
      return line;
    })
    .join("\n");
}

function detailsWidgetValues(
  raw: string,
  result: Record<string, string>,
): void {
  const lines = raw.split("\n");
  const open = lines[0].startsWith("???+");
  const marker = open ? "???+" : "???";
  result.title = parseQuotedTitle(lines[0], marker) || "";
  result.open = open ? "true" : "false";
  result.body = deindentBody(lines.slice(1));
}

function tabsWidgetValues(
  raw: string,
  widget: CatalogWidget,
  result: Record<string, string>,
): void {
  const lines = raw.split("\n");
  const titles: string[] = [];
  const bodies: string[] = [];
  let index = 0;
  while (index < lines.length) {
    if (!lines[index].startsWith('=== "')) break;
    const title = parseQuotedTitle(lines[index], "===");
    if (title === null) break;
    const bodyStart = index + 1;
    const bodyEnd = indentedBlockEnd(lines, bodyStart);
    titles.push(title);
    bodies.push(deindentBody(lines.slice(bodyStart, bodyEnd)));
    index = bodyEnd;
    while (index < lines.length && lines[index].trim() === "") index += 1;
  }
  const titleAttribute = widget.attributes.find(
    (attribute) => attribute.name === "titles",
  );
  const bodyAttribute = widget.attributes.find(
    (attribute) => attribute.name === "bodies",
  );
  result.titles = titles.join(titleAttribute?.separator || "\x1f");
  result.bodies = bodies.join(bodyAttribute?.separator || "\x1f");
}

export function widgetValues(
  raw: string,
  widget: CatalogWidget,
): Record<string, string> {
  const result: Record<string, string> = {};
  for (const attribute of widget.attributes) {
    if (attribute.default !== undefined)
      result[attribute.name] = attribute.default;
  }

  switch (widget.syntax.kind) {
    case "macro":
      macroWidgetValues(raw, widget, result);
      break;
    case "substitution":
      substitutionWidgetValues(raw, widget, result);
      break;
    case "callout":
      calloutWidgetValues(raw, result);
      break;
    case "details":
      detailsWidgetValues(raw, result);
      break;
    case "tabs":
      tabsWidgetValues(raw, widget, result);
      break;
  }
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

function emitKnownMacroAttribute(
  parts: string[],
  declaration: CatalogWidgetAttribute,
  value: string,
): void {
  if (value === "" && !declaration.required && !declaration.emit_empty) return;
  if (
    !declaration.required &&
    declaration.default !== undefined &&
    value === declaration.default
  )
    return;
  if (declaration.repeat) {
    for (const item of splitWidgetList(value, declaration)) {
      if (item) parts.push(`${declaration.name}=${quoteAttribute(item)}`);
    }
    return;
  }
  parts.push(`${declaration.name}=${quoteAttribute(value)}`);
}

// rewriteWidgetMacro updates declared attributes while preserving unknown attributes and their order.
export function rewriteWidgetMacro(
  raw: string,
  widget: CatalogWidget,
  values: Record<string, string>,
): string {
  const macro = parseMacro(raw);
  if (!macro || macro.name !== widget.syntax.name) return raw;
  const known = new Map(
    widget.attributes.map((attribute) => [attribute.name, attribute]),
  );
  const emitted = new Set<string>();
  const parts: string[] = [];

  for (const attribute of macro.attributes) {
    const declaration = known.get(attribute.name);
    if (!declaration) {
      parts.push(`${attribute.name}=${quoteAttribute(attribute.value)}`);
      continue;
    }
    if (emitted.has(attribute.name)) continue;
    emitted.add(attribute.name);
    emitKnownMacroAttribute(parts, declaration, values[attribute.name] ?? "");
  }

  for (const declaration of widget.attributes) {
    if (emitted.has(declaration.name)) continue;
    emitKnownMacroAttribute(parts, declaration, values[declaration.name] ?? "");
  }

  return `{{${macro.name}${parts.length ? " " + parts.join(" ") : ""}}}`;
}

function indentBody(value: string): string {
  return value
    .split("\n")
    .map((line) => (line ? `    ${line}` : ""))
    .join("\n");
}

function rewriteSubstitution(
  widget: CatalogWidget,
  values: Record<string, string>,
): string {
  const declaration = firstWidgetAttribute(widget);
  const value = declaration ? values[declaration.name] || "" : "";
  return `{{${widget.syntax.name}:${value}}}`;
}

function rewriteCallout(values: Record<string, string>): string {
  const kind = values.kind || "note";
  const body = values.body || "";
  return `!!! ${kind}${body ? `\n${body}` : ""}`;
}

function rewriteDetails(values: Record<string, string>): string {
  const marker = values.open === "true" ? "???+" : "???";
  const title = values.title || "Details";
  const body = values.body || "";
  return `${marker} ${JSON.stringify(title)}${body ? `\n\n${indentBody(body)}` : ""}`;
}

function rewriteTabs(
  widget: CatalogWidget,
  values: Record<string, string>,
): string {
  const titlesAttribute = widgetAttribute(widget, "titles");
  const bodiesAttribute = widgetAttribute(widget, "bodies");
  const titles = titlesAttribute
    ? splitWidgetList(values.titles || "", titlesAttribute)
    : [];
  const bodies = bodiesAttribute
    ? splitWidgetList(values.bodies || "", bodiesAttribute)
    : [];
  return titles
    .map((title, index) => {
      const body = bodies[index] || "";
      return `=== ${JSON.stringify(title)}${body ? `\n\n${indentBody(body)}` : ""}`;
    })
    .join("\n\n");
}

// rewriteWidgetSource serializes edited values back to the plugin's original Markdown syntax.
export function rewriteWidgetSource(
  raw: string,
  widget: CatalogWidget,
  values: Record<string, string>,
): string {
  switch (widget.syntax.kind) {
    case "macro":
      return rewriteWidgetMacro(raw, widget, values);
    case "substitution":
      return rewriteSubstitution(widget, values);
    case "callout":
      return rewriteCallout(values);
    case "details":
      return rewriteDetails(values);
    case "tabs":
      return rewriteTabs(widget, values);
  }
}
