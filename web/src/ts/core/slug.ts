// Canonical page paths, matching internal/markdown.Slug.
export function slugifyPagePath(value: string): string {
  let output = "";
  let separator = false;

  for (const source of value.trim()) {
    // Go uses simple, per-rune lowercase mapping. JavaScript's full lowercase
    // mapping expands U+0130 to i + combining dot; its simple mapping is i.
    const character = source === "\u0130" ? "i" : source.toLowerCase();
    if (/^[a-z0-9/_-]$/.test(character)) {
      if (separator && output) output += "-";
      separator = false;
      output += character;
    } else {
      separator = true;
    }
  }

  return output.replace(/^-+|-+$/g, "");
}
