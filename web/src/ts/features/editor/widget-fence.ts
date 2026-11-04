// Generic source mapping for plugin-declared fenced widgets.
export function matchWidgetFence(
  source: string,
  language: string,
): { raw: string; body: string } | null {
  const opening = /^ {0,3}(`{3,}|~{3,})([^\r\n]*)\r?\n/.exec(source);
  if (!opening || opening[2].trim().toLowerCase() !== language.toLowerCase())
    return null;
  const closing = /^ {0,3}(`{3,}|~{3,})[ \t]*(?:\r?$)/gm;
  closing.lastIndex = opening[0].length;
  let match: RegExpExecArray | null;
  while ((match = closing.exec(source))) {
    if (match[1][0] !== opening[1][0] || match[1].length < opening[1].length)
      continue;
    return {
      raw: source.slice(0, match.index + match[0].length),
      body: source.slice(opening[0].length, match.index).replace(/\r?\n$/, ""),
    };
  }
  return null;
}

export function serializeWidgetFence(language: string, body: string): string {
  const longest = Math.max(
    2,
    ...(body.match(/`+/g) ?? []).map((value) => value.length),
  );
  const marker = "`".repeat(longest + 1);
  return `${marker}${language}\n${body}\n${marker}`;
}
