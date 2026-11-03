// Mermaid alignment is stored as an ordinary Mermaid comment so fenced source
// survives Markdown/visual-editor round trips without custom Markdown syntax.
export type MermaidAlignment = "left" | "center" | "right";

const directive =
  /^%%[ \t]+kumbuka-align:[ \t]*(left|center|right)[ \t]*(?:\r?\n|$)/i;

// mermaidAlignment reads a presentation directive at the start of a diagram.
export function mermaidAlignment(source: string): MermaidAlignment {
  const value = directive.exec(source)?.[1]?.toLowerCase();
  return value === "left" || value === "right" ? value : "center";
}

// withMermaidAlignment updates the directive without rewriting diagram syntax.
export function withMermaidAlignment(
  source: string,
  alignment: MermaidAlignment,
): string {
  const content = source.replace(directive, "");
  return alignment === "center"
    ? content
    : `%% kumbuka-align: ${alignment}\n${content}`;
}
