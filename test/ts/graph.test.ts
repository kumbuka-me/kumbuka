import test from "node:test";
import { readFileSync } from "node:fs";
import assert from "node:assert/strict";

import { graphNeighborhood } from "../../web/src/ts/features/graph.ts";

const graph = {
  nodes: [
    { slug: "a", title: "Alpha" },
    { slug: "b", title: "Beta" },
    { slug: "c", title: "Gamma" },
    { slug: "d", title: "Database" },
  ],
  edges: [
    { source: "a", target: "b" },
    { source: "b", target: "c" },
    { source: "d", target: "c" },
  ],
};

test("graphNeighborhood returns the complete graph without a filter", () => {
  assert.deepEqual(graphNeighborhood(graph, "", ""), graph);
});

test("graphNeighborhood returns a focused node and its direct neighbors", () => {
  const result = graphNeighborhood(graph, "b");

  assert.deepEqual(result.nodes.map((node) => node.slug).sort(), [
    "a",
    "b",
    "c",
  ]);
  assert.equal(result.edges.length, 2);
});

test("graphNeighborhood expands matching search results with their relationships", () => {
  const result = graphNeighborhood(graph, "", "database");

  assert.deepEqual(result.nodes.map((node) => node.slug).sort(), ["c", "d"]);
  assert.deepEqual(result.edges, [{ source: "d", target: "c" }]);
});

test("knowledgeGraph rejects malformed graph entries instead of filtering them", async () => {
  const { knowledgeGraph } = await import("../../web/src/ts/features/graph.ts");
  let caught: unknown;

  try {
    knowledgeGraph({
      nodes: [
        { slug: "a", title: "Alpha" },
        { slug: 7, title: "Broken" },
      ],
      edges: [],
    });
  } catch (error) {
    caught = error;
  }

  assert.ok(caught instanceof Error);
  assert.equal(caught.message, "Invalid knowledge graph nodes.");
});

test("knowledgeGraph accepts the empty Go graph contract", async () => {
  const { knowledgeGraph } = await import("../../web/src/ts/features/graph.ts");
  const fixtures = JSON.parse(
    readFileSync("test/contracts/http.json", "utf8"),
  ) as Record<string, unknown>;
  assert.deepEqual(knowledgeGraph(fixtures.empty_graph), {
    nodes: [],
    edges: [],
  });
});
