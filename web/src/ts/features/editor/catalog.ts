// Runtime-checked metadata returned by GET /api/editor/catalog.

import { isRecord, isStringRecord, requireArrayOf } from "../../core/guards.ts";

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
  inline: boolean;
}

export interface EditorCatalog {
  pages: CatalogPage[];
  aliases: Record<string, string>;
  completions: CatalogCompletion[];
  inserts: CatalogInsert[];
}

function isCatalogPage(value: unknown): value is CatalogPage {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    typeof value.title === "string"
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
    inserts: requireArrayOf(
      value.inserts,
      isCatalogInsert,
      "editor catalog inserts",
    ),
    aliases: value.aliases,
  };
}
