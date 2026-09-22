// Pure Markdown syntax helpers shared by the visual editor and unit tests.

// Matches one Kumbuka inline construct without consuming surrounding Markdown.
export function matchKumbukaInlineSyntax(source: string): string | null {
  const macro = /^\{\{[^{}\n]+\}\}/.exec(source);
  const wiki = /^\[\[[^\]\n]+\]\]/.exec(source);

  if (macro) return macro[0];
  if (wiki) return wiki[0];
  return null;
}

// Matches one standalone Kumbuka directive line such as a table directive.
export function matchKumbukaBlockSyntax(source: string): string | null {
  const match = /^\{[A-Za-z][A-Za-z0-9_-]*(?:[ \t]+[^{}\n]*)?\}(?=\n|$)/.exec(
    source,
  );

  return match?.[0] ?? null;
}

// Returns a compact human label for raw Kumbuka syntax shown in visual mode.
export function visualSyntaxLabel(raw: string): string {
  const trimmed = raw.trim();
  const macro = /^\{\{\s*([^\s}]+)/.exec(trimmed);
  if (macro) {
    const name = macro[1] || "macro";
    if (name === "status") return "Status";
    return "Macro · " + name;
  }

  const wiki = /^\[\[([^\]|]+)(?:\|([^\]]+))?\]\]$/.exec(trimmed);
  if (wiki) return (wiki[2] || wiki[1] || "Page").trim();

  const directive = /^\{\s*([^\s}]+)/.exec(trimmed);
  if (directive) {
    const name = directive[1] || "directive";
    if (name === "table") return "Table options";
    return "Directive · " + name;
  }

  return "Kumbuka syntax";
}
