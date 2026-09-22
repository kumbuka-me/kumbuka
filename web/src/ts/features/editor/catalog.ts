// Runtime-checked metadata returned by GET /api/editor/catalog.

import { isRecord, isStringRecord, requireArrayOf } from "../../core/guards.ts";
import { requestJSON } from "../../core/http.ts";
import {
  isCatalogWidget,
  isCatalogWidgetProblem,
  type CatalogWidget,
  type CatalogWidgetProblem,
} from "./widget-contract.ts";

export interface CatalogPage {
  slug: string;
  title: string;
}

export interface CatalogCompletion {
  plugin_id: string;
  module_id: string;
  trigger: string;
  label: string;
  detail?: string;
  replacement: string;
}

export interface CatalogCompletionField {
  id: string;
  name: string;
  type: string;
  required: boolean;
  key: boolean;
  max_bytes?: number;
  options?: string[];
  default?: string;
}

export interface CatalogCompletionProvider {
  plugin_id: string;
  module_id: string;
  resource_id: string;
  resource_name: string;
  trigger: string;
  replacement: string;
  label_field: string;
  detail_field?: string;
  fields: CatalogCompletionField[];
  can_create: boolean;
}

export interface CatalogInsert {
  plugin_id: string;
  module_id: string;
  name: string;
  description?: string;
  markdown: string;
  suffix?: string;
  placeholder?: string;
  mode: string;
  group: string;
  icon?: string;
  completion_module_id?: string;
  inline: boolean;
}

export interface EditorCatalog {
  pages: CatalogPage[];
  aliases: Record<string, string>;
  completions: CatalogCompletion[];
  completion_providers: CatalogCompletionProvider[];
  inserts: CatalogInsert[];
  widgets: CatalogWidget[];
  widget_problems: CatalogWidgetProblem[];
}

function isCatalogPage(value: unknown): value is CatalogPage {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    typeof value.title === "string"
  );
}

function isCatalogCompletionField(
  value: unknown,
): value is CatalogCompletionField {
  return (
    isRecord(value) &&
    typeof value.id === "string" &&
    typeof value.name === "string" &&
    typeof value.type === "string" &&
    typeof value.required === "boolean" &&
    typeof value.key === "boolean" &&
    (value.max_bytes === undefined || typeof value.max_bytes === "number") &&
    (value.options === undefined ||
      (Array.isArray(value.options) &&
        value.options.every((item) => typeof item === "string"))) &&
    (value.default === undefined || typeof value.default === "string")
  );
}

function isCatalogCompletionProvider(
  value: unknown,
): value is CatalogCompletionProvider {
  return (
    isRecord(value) &&
    typeof value.plugin_id === "string" &&
    typeof value.module_id === "string" &&
    typeof value.resource_id === "string" &&
    typeof value.resource_name === "string" &&
    typeof value.trigger === "string" &&
    typeof value.replacement === "string" &&
    typeof value.label_field === "string" &&
    (value.detail_field === undefined ||
      typeof value.detail_field === "string") &&
    Array.isArray(value.fields) &&
    value.fields.every(isCatalogCompletionField) &&
    typeof value.can_create === "boolean"
  );
}

function isCatalogCompletion(value: unknown): value is CatalogCompletion {
  return (
    isRecord(value) &&
    typeof value.plugin_id === "string" &&
    typeof value.module_id === "string" &&
    typeof value.trigger === "string" &&
    typeof value.label === "string" &&
    (value.detail === undefined || typeof value.detail === "string") &&
    typeof value.replacement === "string"
  );
}

function isCatalogInsert(value: unknown): value is CatalogInsert {
  return (
    isRecord(value) &&
    typeof value.plugin_id === "string" &&
    typeof value.module_id === "string" &&
    typeof value.name === "string" &&
    (value.description === undefined ||
      typeof value.description === "string") &&
    typeof value.markdown === "string" &&
    (value.suffix === undefined || typeof value.suffix === "string") &&
    (value.placeholder === undefined ||
      typeof value.placeholder === "string") &&
    typeof value.mode === "string" &&
    typeof value.group === "string" &&
    (value.icon === undefined || typeof value.icon === "string") &&
    (value.completion_module_id === undefined ||
      typeof value.completion_module_id === "string") &&
    typeof value.inline === "boolean"
  );
}

export function parseEditorCatalog(value: unknown): EditorCatalog {
  if (!isRecord(value) || !isStringRecord(value.aliases)) {
    throw new Error("Invalid editor catalog response.");
  }
  return {
    pages: requireArrayOf(value.pages, isCatalogPage, "editor catalog pages"),
    completions: requireArrayOf(
      value.completions,
      isCatalogCompletion,
      "editor catalog completions",
    ),
    completion_providers:
      value.completion_providers === undefined
        ? []
        : requireArrayOf(
            value.completion_providers,
            isCatalogCompletionProvider,
            "editor catalog completion providers",
          ),
    inserts: requireArrayOf(
      value.inserts,
      isCatalogInsert,
      "editor catalog inserts",
    ),
    widgets: requireArrayOf(
      value.widgets ?? [],
      isCatalogWidget,
      "editor catalog widgets",
    ),
    widget_problems: requireArrayOf(
      value.widget_problems ?? [],
      isCatalogWidgetProblem,
      "editor catalog widget problems",
    ),
    aliases: value.aliases,
  };
}

let catalogLoad: Promise<EditorCatalog> | null = null;

// invalidateEditorCatalog forces the next editor feature to fetch fresh catalog data.
export function invalidateEditorCatalog(): void {
  catalogLoad = null;
}

// loadEditorCatalog shares one in-flight/cached catalog request across editor features.
export function loadEditorCatalog(): Promise<EditorCatalog> {
  if (catalogLoad) return catalogLoad;

  catalogLoad = requestJSON("/api/editor/catalog")
    .then(parseEditorCatalog)
    .catch((error) => {
      catalogLoad = null;
      throw error;
    });
  return catalogLoad;
}
