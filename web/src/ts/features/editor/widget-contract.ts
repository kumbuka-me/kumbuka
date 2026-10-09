// Shared declarative visual-editor widget contract and Markdown mapping helpers.

import { isRecord, isStringRecord } from "../../core/guards.ts";

import type {
  CatalogWidget,
  CatalogWidgetAttribute,
  CatalogWidgetBadgePreview,
  CatalogWidgetCalloutPreview,
  CatalogWidgetCardPreview,
  CatalogWidgetConstraint,
  CatalogWidgetChoice,
  CatalogWidgetChoiceSource,
  CatalogWidgetDetailsPreview,
  CatalogWidgetLineAnnotations,
  CatalogWidgetPreview,
  CatalogWidgetProblem,
  CatalogWidgetReferencePreview,
  CatalogWidgetSetting,
  CatalogWidgetSettingColumn,
  CatalogWidgetTabsPreview,
  CatalogWidgetTreeField,
  MatchedWidgetSource,
  ParsedMacro,
  ParsedMacroAttribute,
  WidgetSyntaxKind,
} from "./widget-contract-types.ts";
import { splitWidgetList } from "./widget-contract-validation.ts";

export * from "./widget-contract-types.ts";
export {
  normalizeWidgetColor,
  splitWidgetList,
  validateWidgetValues,
} from "./widget-contract-validation.ts";

const identifier = /^[a-z0-9][a-z0-9._-]{0,127}$/;
const attributeName = /^[A-Za-z][A-Za-z0-9._-]{0,127}$/;
const widgetIdentifier = /^[A-Za-z0-9][A-Za-z0-9._:/-]*$/;

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

function isWidgetChoice(value: unknown): value is CatalogWidgetChoice {
  return (
    isRecord(value) &&
    typeof value.value === "string" &&
    typeof value.label === "string" &&
    optionalBoolean(value.default)
  );
}

function isWidgetChoiceSource(
  value: unknown,
): value is CatalogWidgetChoiceSource {
  return (
    isRecord(value) &&
    optionalString(value.setting_module_id) &&
    optionalString(value.resource_module_id) &&
    typeof value.source_attribute === "string" &&
    typeof value.list_field === "string" &&
    typeof value.value_column === "string" &&
    typeof value.label_column === "string" &&
    optionalString(value.default_column)
  );
}

function isWidgetChoices(
  value: unknown,
): value is Record<string, CatalogWidgetChoice[]> {
  return (
    isRecord(value) &&
    Object.values(value).every(
      (choices) =>
        Array.isArray(choices) &&
        choices.every((choice) => isWidgetChoice(choice)),
    )
  );
}

function isWidgetTreeField(value: unknown): value is CatalogWidgetTreeField {
  return (
    isRecord(value) &&
    typeof value.attribute === "string" &&
    typeof value.label === "string" &&
    (value.type === "text" ||
      value.type === "textarea" ||
      value.type === "mention" ||
      value.type === "date" ||
      value.type === "select") &&
    optionalString(value.placeholder) &&
    (value.suggestions === undefined || strings(value.suggestions)) &&
    (value.choice_source === undefined ||
      isWidgetChoiceSource(value.choice_source)) &&
    (value.choices === undefined || isWidgetChoices(value.choices))
  );
}

function isWidgetSetting(value: unknown): value is CatalogWidgetSetting {
  if (!isRecord(value)) return false;
  const resourceSetting =
    value.type === "resource"
      ? typeof value.completion_module_id === "string" &&
        identifier.test(value.completion_module_id)
      : value.completion_module_id === undefined;
  return (
    resourceSetting &&
    (value.type === "text" ||
      value.type === "textarea" ||
      value.type === "select" ||
      value.type === "resource" ||
      value.type === "mention" ||
      value.type === "date" ||
      value.type === "table" ||
      value.type === "tree") &&
    typeof value.label === "string" &&
    optionalString(value.attribute) &&
    (value.attributes === undefined || strings(value.attributes)) &&
    (value.columns === undefined ||
      (Array.isArray(value.columns) && value.columns.every(isWidgetColumn))) &&
    optionalString(value.row_separator) &&
    optionalString(value.placeholder) &&
    (value.suggestions === undefined || strings(value.suggestions)) &&
    (value.fields === undefined ||
      (Array.isArray(value.fields) && value.fields.every(isWidgetTreeField))) &&
    optionalString(value.id_attribute) &&
    optionalString(value.parent_attribute) &&
    optionalString(value.title_attribute) &&
    optionalString(value.description_attribute) &&
    optionalString(value.empty_value) &&
    optionalString(value.id_prefix) &&
    optionalNumber(value.max_depth) &&
    (value.choice_source === undefined ||
      isWidgetChoiceSource(value.choice_source)) &&
    (value.choices === undefined || isWidgetChoices(value.choices))
  );
}

function isLineAnnotations(
  value: unknown,
): value is CatalogWidgetLineAnnotations {
  return (
    isRecord(value) &&
    typeof value.attribute === "string" &&
    typeof value.line_class === "string" &&
    typeof value.line_number_class === "string"
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
    optionalString(value.body_text) &&
    optionalBoolean(value.rendered) &&
    (value.line_annotations === undefined ||
      isLineAnnotations(value.line_annotations))
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
  const end = indentedBlockEnd(lines, 1);
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
  result.body = deindentBody(lines.slice(1));
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
  return `!!! ${kind}${body ? `\n${indentBody(body)}` : ""}`;
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
