// Canonical Markdown heading-numbering helpers shared by both editor modes.

export function headingNumberingEnabled(form: HTMLFormElement | null): boolean {
  return (
    form?.querySelector<HTMLInputElement>("[data-heading-numbering]")
      ?.checked ?? false
  );
}

export function renumberMarkdownHeadings(source: string): string {
  const lines = source.split(/(\r?\n)/);
  const counters = [0, 0, 0, 0, 0, 0];
  let fence = "";

  for (let index = 0; index < lines.length; index += 2) {
    const line = lines[index];
    const trimmed = line.replace(/^ {0,3}/, "");
    const marker = /^(?:`{3,}|~{3,})/.exec(trimmed)?.[0] ?? "";
    if (fence) {
      if (
        marker.startsWith(fence) &&
        /^[ \t]*$/.test(trimmed.slice(marker.length))
      ) {
        fence = "";
      }
      continue;
    }
    if (marker) {
      if (
        !marker.startsWith("`") ||
        !trimmed.slice(marker.length).includes("`")
      )
        fence = marker;
      continue;
    }

    const match = /^( {0,3})(#{1,6})[ \t]+(.+?)\s*$/.exec(line);
    if (!match || !match[3].trim()) continue;
    const level = match[2].length;
    const title = match[3].replace(/^\d+(?:\.|(?:\.\d+)+)[ \t]+/, "");
    counters[level - 1] += 1;
    counters.fill(0, level);
    const parts = counters.slice(0, level).filter(Boolean);
    const number = `${parts.join(".")}${parts.length === 1 ? "." : ""}`;
    lines[index] = `${match[1]}${match[2]} ${number} ${title}`;
  }

  return lines.join("");
}
