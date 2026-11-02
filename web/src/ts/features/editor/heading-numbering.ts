// Canonical Markdown heading-numbering helpers shared by both editor modes.

export const headingNumberingProperty = "kumbuka.heading-numbering";

export function headingNumberingEnabled(form: HTMLFormElement | null): boolean {
  return form?.querySelector<HTMLInputElement>("[data-heading-numbering]")?.checked ?? false;
}

export function renumberMarkdownHeadings(source: string): string {
  const lines = source.split(/(\n)/);
  const counters = [0, 0, 0, 0, 0, 0];
  let fence = "";

  for (let index = 0; index < lines.length; index += 2) {
    const line = lines[index];
    const trimmed = line.replace(/^[ \t]*/, "");
    const marker = /^(?:`{3,}|~{3,})/.exec(trimmed)?.[0] ?? "";
    if (marker) {
      if (!fence) fence = marker;
      else if (trimmed.startsWith(fence)) fence = "";
      continue;
    }
    if (fence) continue;

    const match = /^(\s{0,3})(#{1,6})[ \t]+(.+?)\s*$/.exec(line);
    if (!match) continue;
    const level = match[2].length;
    const title = match[3].replace(/^\d+(?:\.\d+)*\.?[ \t]+/, "");
    counters[level - 1] += 1;
    counters.fill(0, level);
    const parts = counters.slice(0, level).filter(Boolean);
    const number = `${parts.join(".")}${parts.length === 1 ? "." : ""}`;
    lines[index] = `${match[1]}${match[2]} ${number} ${title}`;
  }

  return lines.join("");
}
