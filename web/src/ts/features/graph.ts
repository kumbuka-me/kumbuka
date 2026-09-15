// Interactive knowledge graph rendering and inspection.

import { requiredAttribute, requiredElement } from "../core/dom.ts";
import { isRecord, requireArrayOf } from "../core/guards.ts";
import { requestJSON } from "../core/http.ts";

type SVGAttributes = Record<string, string | number>;

interface GraphNode {
  slug: string;
  title: string;
  status?: string;
}

interface GraphEdge {
  source: string;
  target: string;
}

export interface KnowledgeGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

interface Position {
  x: number;
  y: number;
}

function svgElement<K extends keyof SVGElementTagNameMap>(
  name: K,
  attributes: SVGAttributes = {},
): SVGElementTagNameMap[K] {
  const element = document.createElementNS("http://www.w3.org/2000/svg", name);

  for (const [key, value] of Object.entries(attributes))
    element.setAttribute(key, String(value));

  return element;
}

function isGraphNode(value: unknown): value is GraphNode {
  return (
    isRecord(value) &&
    typeof value.slug === "string" &&
    typeof value.title === "string" &&
    (value.status === undefined || typeof value.status === "string")
  );
}

function isGraphEdge(value: unknown): value is GraphEdge {
  return (
    isRecord(value) &&
    typeof value.source === "string" &&
    typeof value.target === "string"
  );
}

export function knowledgeGraph(value: unknown): KnowledgeGraph {
  if (!isRecord(value)) throw new Error("Invalid knowledge graph response.");

  return {
    nodes: requireArrayOf(value.nodes, isGraphNode, "knowledge graph nodes"),
    edges: requireArrayOf(value.edges, isGraphEdge, "knowledge graph edges"),
  };
}

// Returns graph nodes connected within the requested depth.
export function graphNeighborhood(
  graph: Partial<KnowledgeGraph> | null | undefined,
  focus: string,
  query = "",
): KnowledgeGraph {
  const nodes = Array.isArray(graph?.nodes) ? graph.nodes : [];
  const edges = Array.isArray(graph?.edges) ? graph.edges : [];
  const normalizedQuery = query.trim().toLocaleLowerCase();
  const matching = new Set(
    nodes
      .filter(
        (node) =>
          !normalizedQuery ||
          `${node.title} ${node.slug}`
            .toLocaleLowerCase()
            .includes(normalizedQuery),
      )
      .map((node) => node.slug),
  );
  const selected = new Set<string>();

  if (focus) selected.add(focus);
  for (const edge of edges) {
    if (focus && (edge.source === focus || edge.target === focus)) {
      selected.add(edge.source);
      selected.add(edge.target);
    }
    if (
      normalizedQuery &&
      (matching.has(edge.source) || matching.has(edge.target))
    ) {
      selected.add(edge.source);
      selected.add(edge.target);
    }
  }

  if (!focus && !normalizedQuery) return { nodes, edges };

  if (normalizedQuery) {
    for (const slug of matching) selected.add(slug);
  }

  const visibleNodes = nodes.filter((node) => selected.has(node.slug));
  const visible = new Set(visibleNodes.map((node) => node.slug));

  return {
    nodes: visibleNodes,
    edges: edges.filter(
      (edge) => visible.has(edge.source) && visible.has(edge.target),
    ),
  };
}

// Wires graph behavior.
function setupGraph(root: HTMLElement): void {
  const graphSVG = requiredElement<SVGSVGElement>(root, "[data-graph-svg]");
  const graphSearch = requiredElement<HTMLInputElement>(
    root,
    "[data-graph-search]",
  );
  const emptyState = requiredElement<HTMLElement>(root, "[data-graph-empty]");
  const inspectorPanel = requiredElement<HTMLElement>(
    root,
    "[data-graph-inspector]",
  );
  const inspectorTitle = requiredElement<HTMLElement>(
    inspectorPanel,
    "[data-graph-title]",
  );
  const inspectorSlug = requiredElement<HTMLElement>(
    inspectorPanel,
    "[data-graph-slug]",
  );
  const inspectorStatus = requiredElement<HTMLElement>(
    inspectorPanel,
    "[data-graph-status]",
  );
  const inspectorDegree = requiredElement<HTMLElement>(
    inspectorPanel,
    "[data-graph-degree]",
  );
  const inspectorOpen = requiredElement<HTMLAnchorElement>(
    inspectorPanel,
    "[data-graph-open]",
  );
  const fitButton = requiredElement<HTMLButtonElement>(
    root,
    "[data-graph-fit]",
  );
  const inspectorClose = requiredElement<HTMLButtonElement>(
    inspectorPanel,
    "[data-graph-inspector-close]",
  );
  const graphURL = requiredAttribute(root, "data-graph-url");
  let graph: KnowledgeGraph = { nodes: [], edges: [] };
  let focus = root.dataset.graphFocus || "";
  let selected = focus;

  // Updates the graph inspector for one node.
  function inspect(node: GraphNode, degree: number): void {
    selected = node.slug;
    inspectorPanel.hidden = false;

    inspectorTitle.textContent = node.title;
    inspectorSlug.textContent = node.slug;
    inspectorStatus.textContent = node.status || "verified";
    inspectorStatus.className = `page-status-badge status-${node.status || "verified"}`;
    inspectorDegree.textContent = `${degree} linked page${degree === 1 ? "" : "s"}`;
    inspectorOpen.href = `/pages/${node.slug}`;

    render();
  }

  // Renders the current graph neighborhood.
  function render(): void {
    const current = graphNeighborhood(graph, focus, graphSearch.value);

    graphSVG.replaceChildren();
    emptyState.hidden = current.nodes.length > 0;
    if (!current.nodes.length) return;

    const width = Math.max(700, graphSVG.clientWidth || 1000);
    const height = Math.max(560, Math.min(780, window.innerHeight - 260));

    graphSVG.setAttribute("viewBox", `0 0 ${width} ${height}`);

    const center: Position = { x: width / 2, y: height / 2 };
    const radius = Math.max(120, Math.min(width, height) * 0.36);
    const positions = new Map<string, Position>();
    const ordered = [...current.nodes].sort((a, b) =>
      a.slug.localeCompare(b.slug),
    );

    if (focus && ordered.some((node) => node.slug === focus)) {
      positions.set(focus, center);

      const others = ordered.filter((node) => node.slug !== focus);

      others.forEach((node, index) => {
        const angle =
          (Math.PI * 2 * index) / Math.max(1, others.length) - Math.PI / 2;
        positions.set(node.slug, {
          x: center.x + Math.cos(angle) * radius,
          y: center.y + Math.sin(angle) * radius,
        });
      });
    } else {
      ordered.forEach((node, index) => {
        const ring =
          ordered.length > 28 && index >= Math.ceil(ordered.length / 2)
            ? 0.62
            : 1;
        const ringIndex =
          ring === 1 ? index : index - Math.ceil(ordered.length / 2);
        const ringCount =
          ring === 1
            ? Math.min(ordered.length, Math.ceil(ordered.length / 2))
            : ordered.length - Math.ceil(ordered.length / 2);
        const angle =
          (Math.PI * 2 * ringIndex) / Math.max(1, ringCount) - Math.PI / 2;

        positions.set(node.slug, {
          x: center.x + Math.cos(angle) * radius * ring,
          y: center.y + Math.sin(angle) * radius * ring,
        });
      });
    }

    const degrees = new Map<string, number>(
      ordered.map((node) => [node.slug, 0]),
    );

    for (const edge of current.edges) {
      degrees.set(edge.source, (degrees.get(edge.source) || 0) + 1);
      degrees.set(edge.target, (degrees.get(edge.target) || 0) + 1);

      const from = positions.get(edge.source);
      const to = positions.get(edge.target);
      if (!from || !to) continue;

      graphSVG.append(
        svgElement("line", {
          x1: from.x,
          y1: from.y,
          x2: to.x,
          y2: to.y,
          class: "graph-edge",
        }),
      );
    }

    for (const node of ordered) {
      const position = positions.get(node.slug);
      if (!position) continue;

      const group = svgElement("g", {
        class: `graph-node status-${node.status || "verified"}${selected === node.slug ? " selected" : ""}`,
        tabindex: "0",
        role: "button",
      });

      group.append(
        svgElement("circle", {
          cx: position.x,
          cy: position.y,
          r: node.slug === focus ? 23 : 17,
        }),
      );

      const label = svgElement("text", {
        x: position.x,
        y: position.y + (node.slug === focus ? 39 : 32),
        "text-anchor": "middle",
      });

      label.textContent =
        node.title.length > 26 ? `${node.title.slice(0, 24)}…` : node.title;
      group.append(label);
      group.addEventListener("click", () =>
        inspect(node, degrees.get(node.slug) || 0),
      );
      group.addEventListener("dblclick", () => {
        window.location.href = `/pages/${node.slug}`;
      });
      group.addEventListener("keydown", (event: KeyboardEvent) => {
        if (event.key === "Enter") inspect(node, degrees.get(node.slug) || 0);
      });
      graphSVG.append(group);
    }
  }

  graphSearch.addEventListener("input", render);
  fitButton.addEventListener("click", () => {
    focus = "";
    selected = "";
    graphSearch.value = "";
    inspectorPanel.hidden = true;
    render();
  });
  inspectorClose.addEventListener("click", () => {
    inspectorPanel.hidden = true;
    selected = "";
    render();
  });
  window.addEventListener("resize", render);

  async function loadGraph(url: string): Promise<void> {
    try {
      graph = knowledgeGraph(await requestJSON(url));
      render();
    } catch (error) {
      console.error("knowledge graph failed", error);
      emptyState.textContent = "The knowledge graph could not be loaded.";
      emptyState.hidden = false;
    }
  }

  void loadGraph(graphURL);
}

// Initializes graph.
export function initGraph(): void {
  const root = document.querySelector<HTMLElement>("[data-knowledge-graph]");
  if (root) setupGraph(root);
}
