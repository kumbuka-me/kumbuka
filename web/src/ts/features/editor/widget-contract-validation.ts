// Widget value normalization and contract validation.

import type {
  CatalogWidget,
  CatalogWidgetAttribute,
} from "./widget-contract-types.ts";

const color = /^#[0-9a-fA-F]{6}$/;
const canonicalMention = /^@[A-Za-z0-9_.-]+$/;
const calendarDate = /^\d{4}-\d{2}-\d{2}$/;
const widgetIdentifier = /^[A-Za-z0-9][A-Za-z0-9._:/-]*$/;

function findWidgetAttribute(
  widget: CatalogWidget,
  name: string,
): CatalogWidgetAttribute | undefined {
  return widget.attributes.find((attribute) => attribute.name === name);
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

// validCalendarDate reports whether a value is a real canonical YYYY-MM-DD date.
function validCalendarDate(value: string): boolean {
  if (!calendarDate.test(value)) return false;
  const parsed = new Date(`${value}T00:00:00Z`);
  return (
    !Number.isNaN(parsed.valueOf()) &&
    parsed.toISOString().slice(0, 10) === value
  );
}

// validateWidgetValues applies contract validation before a NodeView transaction changes source.
export function validateWidgetValues(
  values: Record<string, string>,
  widget: CatalogWidget,
): string[] {
  const errors: string[] = [];
  const treeAttributes = new Set(
    widget.settings
      .filter((setting) => setting.type === "tree")
      .flatMap((setting) => setting.attributes || []),
  );
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
        if (!item && !treeAttributes.has(attribute.name))
          errors.push(`${attribute.name} contains an empty item.`);
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

  for (const setting of widget.settings) {
    if (setting.type === "tree") {
      const idAttribute = findWidgetAttribute(
        widget,
        setting.id_attribute || "",
      );
      const parentAttribute = findWidgetAttribute(
        widget,
        setting.parent_attribute || "",
      );
      const titleAttribute = findWidgetAttribute(
        widget,
        setting.title_attribute || "",
      );
      const ids = idAttribute
        ? splitWidgetList(values[setting.id_attribute || ""] || "", idAttribute)
        : [];
      const parents = parentAttribute
        ? splitWidgetList(
            values[setting.parent_attribute || ""] || "",
            parentAttribute,
          )
        : [];
      const titles = titleAttribute
        ? splitWidgetList(
            values[setting.title_attribute || ""] || "",
            titleAttribute,
          )
        : [];
      const known = new Map<string, number>();
      ids.forEach((id, index) => {
        if (!id) errors.push("Every tree item needs an internal identity.");
        if (known.has(id)) errors.push("Tree item identities must be unique.");
        const parent = parents[index] || "";
        let depth = 0;
        if (parent) {
          const parentDepth = known.get(parent);
          if (parentDepth === undefined)
            errors.push("A parent tree item must appear before its child.");
          else depth = parentDepth + 1;
        }
        if (depth > (setting.max_depth || 16))
          errors.push(
            `Tree nesting may not exceed ${setting.max_depth || 16} levels.`,
          );
        known.set(id, depth);
        if (!(titles[index] || "").trim())
          errors.push("Every tree item needs a title.");
      });
      const emptyValue = setting.empty_value || "";
      for (const field of setting.fields || []) {
        if (field.type !== "mention" && field.type !== "date") continue;
        const attribute = findWidgetAttribute(widget, field.attribute);
        if (!attribute) continue;
        for (const item of splitWidgetList(
          values[field.attribute] || "",
          attribute,
        )) {
          if (!item || (emptyValue && item === emptyValue)) continue;
          if (field.type === "mention" && !canonicalMention.test(item))
            errors.push(`${field.attribute} contains an invalid @mention.`);
          if (field.type === "date" && !validCalendarDate(item))
            errors.push(
              `${field.attribute} contains an invalid YYYY-MM-DD date.`,
            );
        }
      }
      continue;
    }
    const value = setting.attribute ? values[setting.attribute] || "" : "";
    if (!value) continue;
    if (setting.type === "mention" && !canonicalMention.test(value))
      errors.push(`${setting.attribute} must be a canonical @mention.`);
    if (setting.type === "date" && !validCalendarDate(value))
      errors.push(`${setting.attribute} must use YYYY-MM-DD.`);
  }

  for (const constraint of widget.constraints || []) {
    const present = constraint.attributes.filter((name) =>
      Boolean((values[name] || "").trim()),
    );
    if (constraint.kind === "exactly-one" && present.length !== 1)
      errors.push(`Use exactly one of ${constraint.attributes.join(" or ")}.`);
    if (constraint.kind === "same-length") {
      const lengths = constraint.attributes.map((name) => {
        const attribute = findWidgetAttribute(widget, name);
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
      const listAttribute = findWidgetAttribute(widget, listName);
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
