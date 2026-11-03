// Canonical Markdown heading-numbering helpers shared by both editor modes.

export function headingNumberFormat(form: HTMLFormElement | null): string {
  return (
    form?.querySelector<HTMLSelectElement>("[data-heading-numbering]")?.value ??
    ""
  );
}

function formatCounter(value: number, format: string): string {
  if (!format.startsWith("roman")) return String(value);
  const symbols = [
    "I",
    "IV",
    "V",
    "IX",
    "X",
    "XL",
    "L",
    "XC",
    "C",
    "CD",
    "D",
    "CM",
    "M",
  ];
  const values = [1, 4, 5, 9, 10, 40, 50, 90, 100, 400, 500, 900, 1000];
  let result = "";
  for (let i = values.length - 1; i >= 0; i--) {
    result += symbols[i].repeat(Math.floor(value / values[i]));
    value %= values[i];
  }
  return format === "roman-lower" ? result.toLowerCase() : result;
}

export function headingPrefixPattern(format: string): RegExp {
  return format.startsWith("roman")
    ? /^[ivxlcdm]+(?:\.|(?:\.[ivxlcdm]+)+)[ \t]+/i
    : /^\d+(?:\.|(?:\.\d+)+)[ \t]+/;
}

export function nextHeadingNumber(
  counters: number[],
  level: number,
  format: string,
): string {
  counters[level - 1] += 1;
  counters.fill(0, level);
  const parts = counters
    .slice(0, level)
    .filter(Boolean)
    .map((value) => formatCounter(value, format));
  return `${parts.join(".")}${parts.length === 1 ? "." : ""}`;
}

export function renumberMarkdownHeadings(
  source: string,
  format = "true",
  previous = format,
): string {
  const prefix = headingPrefixPattern(previous);
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
    const title = match[3].replace(prefix, "");
    const number = nextHeadingNumber(counters, level, format);
    lines[index] = `${match[1]}${match[2]} ${number} ${title}`;
  }

  return lines.join("");
}
