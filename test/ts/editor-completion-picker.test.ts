import test from "node:test";
import assert from "node:assert/strict";
import {
  completionItemsForProvider,
  completionProviderForInsert,
  filterCompletionItems,
} from "../../web/src/ts/features/editor/completion-picker.ts";
import type {
  CatalogCompletionProvider,
  EditorCatalog,
} from "../../web/src/ts/features/editor/catalog.ts";

const provider: CatalogCompletionProvider = {
  plugin_id: "me.kumbuka.variables",
  module_id: "completion",
  resource_id: "variables",
  resource_name: "Variables",
  trigger: "{{",
  replacement: "{{var:${name}}}",
  label_field: "name",
  detail_field: "description",
  fields: [],
  can_create: false,
};

const catalog: EditorCatalog = {
  pages: [],
  aliases: {},
  completions: [
    {
      plugin_id: "me.kumbuka.variables",
      module_id: "completion",
      trigger: "{{",
      label: "Environment",
      detail: "Deployment environment",
      replacement: "{{var:Environment}}",
    },
    {
      plugin_id: "me.kumbuka.variables",
      module_id: "completion",
      trigger: "{{",
      label: "Region",
      detail: "Cloud location",
      replacement: "{{var:Region}}",
    },
    {
      plugin_id: "io.example.other",
      module_id: "completion",
      trigger: "{{",
      label: "Other",
      replacement: "{{other}}",
    },
  ],
  completion_providers: [provider],
  inserts: [],
  widgets: [],
  widget_problems: [],
};

test("completion picker resolves only inserts linked to a provider module", () => {
  assert.equal(
    completionProviderForInsert(catalog, {
      pluginID: "me.kumbuka.variables",
      name: "Variable",
      completionModuleID: "completion",
      markdown: "{{",
      inline: true,
    }),
    provider,
  );
  assert.equal(
    completionProviderForInsert(catalog, {
      pluginID: "me.kumbuka.variables",
      markdown: "{{",
    }),
    undefined,
  );
});

test("completion picker scopes items to the matched provider", () => {
  assert.deepEqual(
    completionItemsForProvider(catalog, provider).map((item) => item.label),
    ["Environment", "Region"],
  );
});

test("completion picker filters labels and details as the user types", () => {
  const items = completionItemsForProvider(catalog, provider);

  assert.deepEqual(
    filterCompletionItems(items, "reg").map((item) => item.label),
    ["Region"],
  );
  assert.deepEqual(
    filterCompletionItems(items, "deployment").map((item) => item.label),
    ["Environment"],
  );
  assert.deepEqual(
    filterCompletionItems(items, "missing").map((item) => item.label),
    [],
  );
});
