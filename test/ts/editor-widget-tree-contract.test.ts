import assert from "node:assert/strict";
import test from "node:test";

import {
  isCatalogWidget,
  validateWidgetValues,
  type CatalogWidget,
} from "../../web/src/ts/features/editor/widget-contract.ts";

const separator = "\x1f";

function taskWidget(): CatalogWidget {
  return {
    plugin_id: "me.kumbuka.tasks",
    id: "tasks",
    name: "Tasks",
    inline: false,
    syntax: { kind: "macro", name: "tasks" },
    attributes: [
      {
        name: "texts",
        type: "list",
        required: true,
        max_bytes: 512,
        max_items: 128,
        separator,
      },
      {
        name: "descriptions",
        type: "list",
        max_bytes: 4096,
        max_items: 128,
        separator,
      },
      {
        name: "ids",
        type: "list",
        required: true,
        max_bytes: 128,
        max_items: 128,
        separator,
        unique: true,
      },
      {
        name: "parents",
        type: "list",
        max_bytes: 128,
        max_items: 128,
        separator,
      },
      {
        name: "assignees",
        type: "list",
        max_bytes: 128,
        max_items: 128,
        separator,
      },
      {
        name: "dues",
        type: "list",
        max_bytes: 10,
        max_items: 128,
        separator,
      },
    ],
    settings: [
      {
        type: "tree",
        label: "Tasks",
        attributes: [
          "texts",
          "descriptions",
          "ids",
          "parents",
          "assignees",
          "dues",
        ],
        id_attribute: "ids",
        parent_attribute: "parents",
        title_attribute: "texts",
        description_attribute: "descriptions",
        id_prefix: "task-",
        max_depth: 16,
        fields: [
          { attribute: "texts", label: "Task", type: "text" },
          {
            attribute: "descriptions",
            label: "Description",
            type: "textarea",
          },
          { attribute: "assignees", label: "Assignee", type: "mention" },
          { attribute: "dues", label: "Due", type: "date" },
        ],
      },
    ],
    constraints: [{ kind: "same-length", attributes: ["texts", "ids"] }],
    preview: { kind: "card", card: { class: "task-list", title: "Tasks" } },
  };
}

test("tree widget accepts blank optional cells and nested parent IDs", () => {
  const widget = taskWidget();
  assert.equal(isCatalogWidget(widget), true);
  assert.deepEqual(
    validateWidgetValues(
      {
        texts: `Release${separator}Deploy${separator}Verify`,
        descriptions: `Prepare release${separator}${separator}Check health`,
        ids: `release${separator}deploy${separator}verify`,
        parents: `${separator}release${separator}deploy`,
        assignees: `@alice${separator}${separator}@bob`,
        dues: `2026-10-01${separator}${separator}2026-10-02`,
      },
      widget,
    ),
    [],
  );
});

test("tree widget rejects a parent that does not precede its child", () => {
  const widget = taskWidget();
  const errors = validateWidgetValues(
    {
      texts: `Child${separator}Parent`,
      ids: `child${separator}parent`,
      parents: `parent${separator}`,
    },
    widget,
  );
  assert.ok(errors.some((error) => error.includes("parent tree item")));
});
