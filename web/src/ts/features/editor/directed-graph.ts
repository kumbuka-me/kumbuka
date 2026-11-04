// DirectedGraph's fenced source is the representation of a forward-only directed graph.
export interface DirectedGraphRoute {
  label: string;
  target: string;
}

export interface DirectedGraphStep {
  id: string;
  title: string;
  routes: DirectedGraphRoute[];
}

// Parse source without discarding incomplete routes that the user can repair.
export function parseDirectedGraph(source: string): DirectedGraphStep[] {
  const steps: DirectedGraphStep[] = [];
  const ids = new Set<string>();
  const lines = source.trimEnd().split(/\r?\n/);
  if (lines.length > 512) throw new Error("Use at most 512 lines per graph.");
  for (const [index, line] of lines.entries()) {
    if (!line.trim()) continue;
    const header = /^([a-z][a-z0-9_-]*):\s*(.+)$/i.exec(line);
    if (header) {
      if (ids.has(header[1]))
        throw new Error(`Line ${index + 1}: duplicate step ${header[1]}.`);
      ids.add(header[1]);
      steps.push({ id: header[1], title: header[2], routes: [] });
      continue;
    }
    const route = /^\s+(.*?)\s*->\s*([a-z][a-z0-9_-]*)\s*$/i.exec(line);
    if (!route || !steps.length)
      throw new Error(
        `Line ${index + 1}: use “id: Title” or an indented “Answer -> id”.`,
      );
    steps[steps.length - 1].routes.push({
      label: route[1].trim(),
      target: route[2],
    });
  }
  if (!steps.length) throw new Error("Add a starting step.");
  return steps;
}

export function serializeDirectedGraph(steps: DirectedGraphStep[]): string {
  return steps
    .map(
      (step) =>
        `${step.id}: ${step.title.trim()}${step.routes.map((route) => `\n  ${route.label.trim() ? route.label.trim() + " " : ""}-> ${route.target}`).join("")}`,
    )
    .join("\n");
}

// Match the plugin's forward-only, fully reachable graph contract.
export function graphProblem(steps: DirectedGraphStep[]): string {
  if (!steps.length) return "Add a starting step.";
  const positions = new Map(steps.map((step, index) => [step.id, index]));
  const reached = new Set([steps[0].id]);
  for (const [index, step] of steps.entries()) {
    if (!step.title.trim()) return `Give step ${index + 1} a title.`;
    if (!reached.has(step.id))
      return `Connect a previous step to “${step.title}”.`;
    const labels = new Set<string>();
    for (const route of step.routes) {
      const target = positions.get(route.target);
      if (target === undefined)
        return `Choose a destination for “${step.title}”.`;
      if (target <= index)
        return `“${step.title}” must connect to a later step. Move its destination down.`;
      const label = route.label.trim();
      if (step.routes.length > 1 && !label)
        return `Label every branch of “${step.title}”.`;
      if (labels.has(label))
        return `Use distinct branch labels in “${step.title}”.`;
      labels.add(label);
      reached.add(route.target);
    }
  }
  if (serializeDirectedGraph(steps).split("\n").length > 512)
    return "Use at most 512 lines per graph.";
  return "";
}
