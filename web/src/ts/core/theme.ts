// Theme selection and persistence.

import { requiredElement } from "./dom.ts";
import { isRecord, isStringRecord, requireArrayOf } from "./guards.ts";

type ColorScheme = "light" | "dark";

export interface ThemeDefinition {
  title: string;
  color_scheme: ColorScheme;
  colors: Record<string, string>;
}

function isColorScheme(value: unknown): value is ColorScheme {
  return value === "light" || value === "dark";
}

function isThemeDefinition(value: unknown): value is ThemeDefinition {
  return (
    isRecord(value) &&
    typeof value.title === "string" &&
    value.title.trim() !== "" &&
    isColorScheme(value.color_scheme) &&
    isStringRecord(value.colors)
  );
}

export function parseThemeCatalog(source: string): ThemeDefinition[] {
  let value: unknown;

  try {
    value = JSON.parse(source) as unknown;
  } catch (error) {
    throw new Error("Invalid theme catalog JSON.", { cause: error });
  }

  return requireArrayOf(value, isThemeDefinition, "theme catalog");
}

function findTheme(
  catalog: readonly ThemeDefinition[],
  title: string,
): ThemeDefinition | undefined {
  const normalized = title.toLocaleLowerCase();
  return catalog.find(
    (candidate) => candidate.title.toLocaleLowerCase() === normalized,
  );
}

function applyTheme(
  catalog: readonly ThemeDefinition[],
  select: HTMLSelectElement | null,
  fallbackTitle: string,
  title: string,
): void {
  const theme =
    findTheme(catalog, title) ??
    findTheme(catalog, fallbackTitle) ??
    catalog[0];
  if (!theme) throw new Error("Theme catalog is empty.");

  for (const [key, value] of Object.entries(theme.colors)) {
    document.documentElement.style.setProperty(
      `--${key.replaceAll("_", "-")}`,
      value,
    );
  }

  document.documentElement.style.colorScheme = theme.color_scheme;
  document.documentElement.dataset.theme = theme.title;

  if (select) select.value = theme.title;
}

// Initializes theme.
export function initTheme(): void {
  const source = requiredElement<HTMLScriptElement>(
    document,
    "#kumbuka-themes",
  );
  const catalog = parseThemeCatalog(source.textContent ?? "");
  const select = document.querySelector<HTMLSelectElement>(
    "[data-theme-select]",
  );
  const activeTheme = document.documentElement.dataset.theme || "Light";

  applyTheme(catalog, select, activeTheme, activeTheme);
  select?.addEventListener("change", () =>
    applyTheme(catalog, select, activeTheme, select.value),
  );
}
