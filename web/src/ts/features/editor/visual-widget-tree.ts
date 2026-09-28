// Hierarchical controls for plugin-provided visual-editor widget forms.

import { requestConfirmation } from "../../core/dialogs.ts";
import { createLabel, setupMentionSetting } from "./visual-widget-controls.ts";
import {
  splitWidgetList,
  widgetAttribute,
  type CatalogWidget,
  type CatalogWidgetSetting,
} from "./widget-contract.ts";

let treeWidgetMentionSequence = 0;

interface TreeDragState {
  dragged: HTMLElement | null;
}

function treeListValues(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): Record<string, string[]> {
  const result: Record<string, string[]> = {};
  for (const name of setting.attributes || []) {
    const attribute = widgetAttribute(widget, name);
    result[name] = attribute
      ? splitWidgetList(values[name] || "", attribute)
      : [];
  }
  return result;
}

function generateTreeID(
  setting: CatalogWidgetSetting,
  used: Set<string>,
): string {
  const prefix = setting.id_prefix || "item-";
  for (let attempt = 0; attempt < 16; attempt += 1) {
    let suffix = "";
    if (globalThis.crypto?.randomUUID) {
      suffix = globalThis.crypto.randomUUID().replaceAll("-", "");
    } else if (globalThis.crypto?.getRandomValues) {
      const bytes = new Uint8Array(12);
      globalThis.crypto.getRandomValues(bytes);
      suffix = Array.from(bytes, (value) =>
        value.toString(16).padStart(2, "0"),
      ).join("");
    } else {
      suffix = `${Date.now().toString(36)}${attempt.toString(36)}`;
    }
    const id = `${prefix}${suffix}`.slice(0, 128);
    if (!used.has(id)) return id;
  }
  return `${prefix}${Date.now().toString(36)}`.slice(0, 128);
}

function directTreeParent(item: HTMLElement): HTMLElement | null {
  const container = item.parentElement;
  if (!container?.classList.contains("visual-widget-tree-children"))
    return null;
  return container.closest<HTMLElement>(".visual-widget-tree-item");
}

function treeDepth(item: HTMLElement): number {
  let depth = 0;
  let parent = directTreeParent(item);
  while (parent) {
    depth += 1;
    parent = directTreeParent(parent);
  }
  return depth;
}

function directTreeChildren(item: HTMLElement): HTMLElement[] {
  const container = item.querySelector<HTMLElement>(
    ":scope > .visual-widget-tree-children",
  );
  if (!container) return [];
  return Array.from(container.children).filter(
    (child): child is HTMLElement =>
      child instanceof HTMLElement &&
      child.classList.contains("visual-widget-tree-item"),
  );
}

function treeSubtreeHeight(item: HTMLElement): number {
  const children = directTreeChildren(item);
  if (!children.length) return 0;
  return 1 + Math.max(...children.map(treeSubtreeHeight));
}

function canPlaceTreeItem(
  item: HTMLElement,
  parent: HTMLElement | null,
  maxDepth: number,
): boolean {
  if (parent && (parent === item || item.contains(parent))) return false;
  const parentDepth = parent ? treeDepth(parent) : -1;
  return parentDepth + 1 + treeSubtreeHeight(item) <= maxDepth;
}

function treeMetadataText(item: HTMLElement): string {
  const values: string[] = [];
  for (const control of item.querySelectorAll<
    HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement
  >(
    ":scope > .visual-widget-tree-card > .visual-widget-tree-editor [data-widget-tree-attribute]",
  )) {
    const type = control.dataset.widgetTreeFieldType || "";
    if (type === "textarea" || control.dataset.widgetTreeTitle === "") continue;
    const value = control.value.trim();
    if (value) values.push(value);
  }
  return values.join(" · ");
}

function updateTreeItemSummary(
  item: HTMLElement,
  setting: CatalogWidgetSetting,
): void {
  const titleControl = item.querySelector<
    HTMLInputElement | HTMLTextAreaElement
  >(
    `[data-widget-tree-attribute="${CSS.escape(setting.title_attribute || "")}"]`,
  );
  const title = item.querySelector<HTMLElement>(".visual-widget-tree-title");
  if (title) title.textContent = titleControl?.value.trim() || "New task";

  const descriptionName = setting.description_attribute || "";
  const descriptionControl = descriptionName
    ? item.querySelector<HTMLInputElement | HTMLTextAreaElement>(
        `[data-widget-tree-attribute="${CSS.escape(descriptionName)}"]`,
      )
    : null;
  const description = item.querySelector<HTMLElement>(
    ".visual-widget-tree-description",
  );
  if (description) {
    description.textContent = descriptionControl?.value.trim() || "";
    description.hidden = !description.textContent;
  }

  const metadata = item.querySelector<HTMLElement>(
    ".visual-widget-tree-metadata",
  );
  if (metadata) {
    metadata.textContent = treeMetadataText(item);
    metadata.hidden = !metadata.textContent;
  }
}

function createTreeControl(
  field: NonNullable<CatalogWidgetSetting["fields"]>[number],
  widget: CatalogWidget,
  value: string,
  titleAttribute: string,
): HTMLElement {
  const wrapper = createLabel(field.label);
  wrapper.classList.add("visual-widget-tree-field-control");
  if (field.attribute === titleAttribute)
    wrapper.classList.add("visual-widget-tree-field-title");
  if (field.type === "textarea")
    wrapper.classList.add("visual-widget-tree-field-description");

  let control: HTMLInputElement | HTMLTextAreaElement;
  if (field.type === "textarea") {
    const textarea = document.createElement("textarea");
    textarea.rows = 3;
    textarea.placeholder = field.placeholder || "";
    control = textarea;
  } else {
    const input = document.createElement("input");
    input.type = field.type === "date" ? "date" : "text";
    input.placeholder = field.placeholder || "";
    if (field.type === "mention") setupMentionSetting(input, wrapper);
    if (field.suggestions?.length) {
      const list = document.createElement("datalist");
      list.id = `visual-widget-tree-suggestions-${widget.id}-${++treeWidgetMentionSequence}`;
      for (const suggestion of field.suggestions) {
        const option = document.createElement("option");
        option.value = suggestion;
        list.append(option);
      }
      input.setAttribute("list", list.id);
      wrapper.append(list);
    }
    control = input;
  }
  control.value = value;
  control.dataset.widgetTreeAttribute = field.attribute;
  control.dataset.widgetTreeFieldType = field.type;
  if (field.attribute === titleAttribute) {
    control.required = true;
    control.dataset.widgetTreeTitle = "";
  }
  wrapper.append(control);
  return wrapper;
}

function setupTreeDropContainer(
  container: HTMLElement,
  parent: HTMLElement | null,
  state: TreeDragState,
  maxDepth: number,
): void {
  container.addEventListener("dragover", (event) => {
    const dragged = state.dragged;
    if (!dragged || !canPlaceTreeItem(dragged, parent, maxDepth)) return;
    event.preventDefault();
    container.classList.add("visual-widget-tree-drop-target");
  });
  container.addEventListener("dragleave", (event) => {
    if (
      event.relatedTarget instanceof Node &&
      container.contains(event.relatedTarget)
    )
      return;
    container.classList.remove("visual-widget-tree-drop-target");
  });
  container.addEventListener("drop", (event) => {
    const dragged = state.dragged;
    container.classList.remove("visual-widget-tree-drop-target");
    if (!dragged || !canPlaceTreeItem(dragged, parent, maxDepth)) return;
    event.preventDefault();
    event.stopPropagation();
    container.append(dragged);
  });
}

function appendTreeItem(
  container: HTMLElement,
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  itemValues: Record<string, string>,
  id: string,
  state: TreeDragState,
  used: Set<string>,
  expanded: boolean,
): HTMLElement {
  const item = document.createElement("article");
  item.className = "visual-widget-tree-item";
  item.dataset.widgetTreeId = id;

  const card = document.createElement("div");
  card.className = "visual-widget-tree-card";
  const header = document.createElement("div");
  header.className = "visual-widget-tree-item-header";

  const handle = document.createElement("button");
  handle.type = "button";
  handle.className = "visual-widget-tree-drag";
  handle.textContent = "⋮⋮";
  handle.title = "Drag to reorder or move";
  handle.setAttribute("aria-label", "Drag task to reorder or move");

  const stateIcon = document.createElement("span");
  stateIcon.className = "visual-widget-tree-state";
  stateIcon.textContent = "○";
  stateIcon.setAttribute("aria-hidden", "true");

  const summary = document.createElement("button");
  summary.type = "button";
  summary.className = "visual-widget-tree-summary";
  const title = document.createElement("strong");
  title.className = "visual-widget-tree-title";
  const description = document.createElement("span");
  description.className = "visual-widget-tree-description";
  const metadata = document.createElement("span");
  metadata.className = "visual-widget-tree-metadata";
  summary.append(title, description, metadata);

  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "visual-widget-tree-toggle";
  toggle.setAttribute("aria-label", "Expand task editor");

  const remove = document.createElement("button");
  remove.type = "button";
  remove.className = "visual-widget-tree-remove";
  remove.textContent = "×";
  remove.title = "Delete task";
  remove.setAttribute("aria-label", "Delete task");

  header.append(handle, stateIcon, summary, toggle, remove);

  const editor = document.createElement("div");
  editor.className = "visual-widget-tree-editor";
  const fields = document.createElement("div");
  fields.className = "visual-widget-tree-fields";
  for (const field of setting.fields || [])
    fields.append(
      createTreeControl(
        field,
        widget,
        itemValues[field.attribute] || "",
        setting.title_attribute || "",
      ),
    );
  editor.append(fields);

  const addChild = document.createElement("button");
  addChild.type = "button";
  addChild.className = "visual-widget-tree-add-child";
  addChild.textContent = "+ Add subtask";
  editor.append(addChild);

  const children = document.createElement("div");
  children.className = "visual-widget-tree-children";
  setupTreeDropContainer(children, item, state, setting.max_depth || 16);

  card.append(header, editor);
  item.append(card, children);
  container.append(item);

  const setExpanded = (value: boolean) => {
    editor.hidden = !value;
    item.classList.toggle("visual-widget-tree-item-expanded", value);
    toggle.textContent = value ? "▾" : "▸";
    toggle.setAttribute(
      "aria-label",
      value ? "Collapse task editor" : "Expand task editor",
    );
    summary.setAttribute("aria-expanded", String(value));
  };
  setExpanded(expanded);
  summary.addEventListener("click", () => setExpanded(Boolean(editor.hidden)));
  toggle.addEventListener("click", () => setExpanded(Boolean(editor.hidden)));

  for (const control of editor.querySelectorAll<
    HTMLInputElement | HTMLTextAreaElement
  >("[data-widget-tree-attribute]")) {
    control.addEventListener("input", () =>
      updateTreeItemSummary(item, setting),
    );
  }

  addChild.addEventListener("click", () => {
    const maxDepth = setting.max_depth || 16;
    if (treeDepth(item) >= maxDepth) return;
    const childID = generateTreeID(setting, used);
    used.add(childID);
    const childValues: Record<string, string> = {};
    appendTreeItem(
      children,
      setting,
      widget,
      childValues,
      childID,
      state,
      used,
      true,
    )
      .querySelector<HTMLInputElement>(
        `[data-widget-tree-attribute="${CSS.escape(setting.title_attribute || "")}"]`,
      )
      ?.focus();
  });

  remove.addEventListener("click", () => {
    const descendants = item.querySelectorAll(
      ".visual-widget-tree-item",
    ).length;
    const removeItem = () => {
      used.delete(item.dataset.widgetTreeId || "");
      for (const child of item.querySelectorAll<HTMLElement>(
        ".visual-widget-tree-item",
      ))
        used.delete(child.dataset.widgetTreeId || "");
      item.remove();
    };
    if (!descendants) {
      removeItem();
      return;
    }
    void requestConfirmation(
      `Delete this task and its ${descendants} subtask${descendants === 1 ? "" : "s"}?`,
      {
        eyebrow: "Delete task",
        title: "Delete task and subtasks?",
        confirmLabel: "Delete",
        cancelLabel: "Keep task",
        danger: true,
      },
    ).then((confirmed) => {
      if (confirmed) removeItem();
    });
  });

  handle.addEventListener("pointerdown", () => {
    item.draggable = true;
  });
  handle.addEventListener("pointerup", () => {
    item.draggable = false;
  });
  item.addEventListener("dragstart", (event) => {
    event.stopPropagation();
    state.dragged = item;
    item.classList.add("visual-widget-tree-dragging");
    event.dataTransfer?.setData("text/plain", id);
    if (event.dataTransfer) event.dataTransfer.effectAllowed = "move";
  });
  item.addEventListener("dragend", (event) => {
    event.stopPropagation();
    item.draggable = false;
    item.classList.remove("visual-widget-tree-dragging");
    state.dragged = null;
    document
      .querySelectorAll(".visual-widget-tree-drop-target")
      .forEach((element) =>
        element.classList.remove("visual-widget-tree-drop-target"),
      );
  });
  header.addEventListener("dragover", (event) => {
    const dragged = state.dragged;
    const parent = directTreeParent(item);
    if (
      !dragged ||
      dragged === item ||
      !canPlaceTreeItem(dragged, parent, setting.max_depth || 16)
    )
      return;
    event.preventDefault();
    header.classList.add("visual-widget-tree-drop-before");
  });
  header.addEventListener("dragleave", () =>
    header.classList.remove("visual-widget-tree-drop-before"),
  );
  header.addEventListener("drop", (event) => {
    const dragged = state.dragged;
    header.classList.remove("visual-widget-tree-drop-before");
    const parent = directTreeParent(item);
    if (
      !dragged ||
      dragged === item ||
      !canPlaceTreeItem(dragged, parent, setting.max_depth || 16)
    )
      return;
    event.preventDefault();
    event.stopPropagation();
    item.before(dragged);
  });

  updateTreeItemSummary(item, setting);
  return item;
}

export function createTreeSetting(
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  values: Record<string, string>,
): HTMLElement {
  const field = document.createElement("fieldset");
  field.className = "visual-widget-tree-field";
  field.dataset.widgetTreeSetting = setting.id_attribute || setting.label;
  const legend = document.createElement("legend");
  const legendTitle = document.createElement("span");
  legendTitle.textContent = setting.label;
  const count = document.createElement("span");
  count.className = "visual-widget-tree-count";
  legend.append(legendTitle, count);
  field.append(legend);

  const root = document.createElement("div");
  root.className = "visual-widget-tree-list";
  root.dataset.widgetTreeRoot = "";
  field.append(root);

  const addRoot = document.createElement("button");
  addRoot.type = "button";
  addRoot.className = "visual-widget-tree-add-root";
  addRoot.textContent = "+ Add task";
  field.append(addRoot);

  const state: TreeDragState = { dragged: null };
  setupTreeDropContainer(root, null, state, setting.max_depth || 16);
  const lists = treeListValues(setting, widget, values);
  const idName = setting.id_attribute || "";
  const parentName = setting.parent_attribute || "";
  const titleName = setting.title_attribute || "";
  const countRows = Math.max(
    lists[idName]?.length || 0,
    lists[titleName]?.length || 0,
    ...Object.values(lists).map((list) => list.length),
  );
  const used = new Set<string>();
  const items = new Map<string, HTMLElement>();
  const rows: Array<{
    id: string;
    parent: string;
    values: Record<string, string>;
  }> = [];

  for (let index = 0; index < countRows; index += 1) {
    let id = (lists[idName]?.[index] || "").trim();
    if (!id) id = generateTreeID(setting, used);
    used.add(id);
    const rowValues: Record<string, string> = {};
    for (const itemField of setting.fields || [])
      rowValues[itemField.attribute] =
        lists[itemField.attribute]?.[index] || "";
    rows.push({
      id,
      parent: (lists[parentName]?.[index] || "").trim(),
      values: rowValues,
    });
  }

  rows.forEach((row, index) => {
    const item = appendTreeItem(
      root,
      setting,
      widget,
      row.values,
      row.id,
      state,
      used,
      index === 0,
    );
    items.set(row.id, item);
  });
  rows.forEach((row) => {
    if (!row.parent) return;
    const item = items.get(row.id);
    const parent = items.get(row.parent);
    const children = parent?.querySelector<HTMLElement>(
      ":scope > .visual-widget-tree-children",
    );
    if (
      item &&
      parent &&
      children &&
      canPlaceTreeItem(item, parent, setting.max_depth || 16)
    )
      children.append(item);
  });

  const updateCount = () => {
    const total = root.querySelectorAll(".visual-widget-tree-item").length;
    count.textContent = `${total} task${total === 1 ? "" : "s"}`;
  };
  updateCount();
  const observer = new MutationObserver(updateCount);
  observer.observe(root, { childList: true, subtree: true });

  addRoot.addEventListener("click", () => {
    const id = generateTreeID(setting, used);
    used.add(id);
    appendTreeItem(root, setting, widget, {}, id, state, used, true)
      .querySelector<HTMLInputElement>(
        `[data-widget-tree-attribute="${CSS.escape(titleName)}"]`,
      )
      ?.focus();
  });
  return field;
}

export function collectTreeValues(
  form: HTMLFormElement,
  setting: CatalogWidgetSetting,
  widget: CatalogWidget,
  result: Record<string, string>,
): void {
  const field = form.querySelector<HTMLElement>(
    `[data-widget-tree-setting="${CSS.escape(setting.id_attribute || setting.label)}"]`,
  );
  if (!field) return;
  const items = Array.from(
    field.querySelectorAll<HTMLElement>(".visual-widget-tree-item"),
  );
  const rows: Record<string, string[]> = {};
  for (const name of setting.attributes || []) rows[name] = [];

  for (const item of items) {
    const id = item.dataset.widgetTreeId || "";
    const parent = directTreeParent(item)?.dataset.widgetTreeId || "";
    if (setting.id_attribute) rows[setting.id_attribute]?.push(id);
    if (setting.parent_attribute) rows[setting.parent_attribute]?.push(parent);
    for (const treeField of setting.fields || []) {
      const control = item.querySelector<
        HTMLInputElement | HTMLTextAreaElement
      >(
        `:scope > .visual-widget-tree-card > .visual-widget-tree-editor [data-widget-tree-attribute="${CSS.escape(treeField.attribute)}"]`,
      );
      rows[treeField.attribute]?.push(control?.value.trim() || "");
    }
    for (const name of setting.attributes || []) {
      if (
        name === setting.id_attribute ||
        name === setting.parent_attribute ||
        (setting.fields || []).some((treeField) => treeField.attribute === name)
      )
        continue;
      rows[name]?.push("");
    }
  }

  for (const name of setting.attributes || []) {
    const attribute = widgetAttribute(widget, name);
    if (!attribute) continue;
    const values = rows[name] || [];
    result[name] = values.every((value) => !value)
      ? ""
      : values.join(attribute.separator || ";");
  }
}

export function treeFormProblem(form: HTMLFormElement, widget: CatalogWidget): string {
  for (const setting of widget.settings) {
    if (setting.type !== "tree") continue;
    const field = form.querySelector<HTMLElement>(
      `[data-widget-tree-setting="${CSS.escape(setting.id_attribute || setting.label)}"]`,
    );
    if (!field) continue;
    const items = Array.from(
      field.querySelectorAll<HTMLElement>(".visual-widget-tree-item"),
    );
    if (!items.length)
      return `Add at least one ${setting.label.toLocaleLowerCase()} item.`;
    const ids = new Set<string>();
    for (const item of items) {
      const id = item.dataset.widgetTreeId || "";
      if (!id || ids.has(id))
        return "Task identities are invalid; remove and recreate the affected task.";
      ids.add(id);
      const title = item.querySelector<HTMLInputElement | HTMLTextAreaElement>(
        `[data-widget-tree-attribute="${CSS.escape(setting.title_attribute || "")}"]`,
      );
      if (!title?.value.trim()) return "Every task needs a title.";
      if (treeDepth(item) > (setting.max_depth || 16))
        return `Task nesting may not exceed ${setting.max_depth || 16} levels.`;
    }
  }
  return "";
}
