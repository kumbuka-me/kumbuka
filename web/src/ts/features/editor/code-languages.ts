// Language definitions and aliases belong to enabled plugins.
export interface CodeLanguage {
  label: string;
  value: string;
  aliases?: string[];
}

export function canonicalCodeLanguage(
  value: string,
  languages: CodeLanguage[] = [],
): string {
  const normalized = value.trim().toLowerCase();
  if (["plain", "plaintext", "text"].includes(normalized)) return "";
  return (
    languages.find((language) => language.value === normalized)?.value ??
    languages.find((language) => language.aliases?.includes(normalized))
      ?.value ??
    normalized
  );
}

export function supportedCodeLanguages(
  languages: CodeLanguage[] = [],
): CodeLanguage[] {
  const unique = new Map(
    languages.map((language) => [language.value, language]),
  );
  unique.delete("");
  return [{ label: "Plain text", value: "" }, ...unique.values()];
}

export function codeLanguageLabel(
  value: string,
  languages: CodeLanguage[] = [],
): string {
  const canonical = canonicalCodeLanguage(value, languages);
  if (!canonical) return "Plain text";
  return (
    languages.find((language) => language.value === canonical)?.label ?? value
  );
}

export function filterCodeLanguages(
  query: string,
  languages: CodeLanguage[] = [],
): CodeLanguage[] {
  const normalized = query.trim().toLowerCase();
  return supportedCodeLanguages(languages)
    .filter(
      (language) =>
        language.label.toLowerCase().includes(normalized) ||
        language.value.includes(normalized) ||
        language.aliases?.some((alias) => alias.includes(normalized)),
    )
    .sort((a, b) => a.label.localeCompare(b.label));
}

export function pluginCodeLanguages(
  catalog: { code_languages?: CodeLanguage[] } | null,
): CodeLanguage[] {
  return (catalog?.code_languages ?? []).map(({ value, label, aliases }) => ({
    value,
    label,
    aliases,
  }));
}
