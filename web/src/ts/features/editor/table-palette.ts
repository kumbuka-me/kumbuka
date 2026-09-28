import { requiredElement, requiredElements } from "../../core/dom.ts";
import {
  clearMarkdownTableCell,
  cloneDirective,
  deleteMarkdownTable,
  deleteMarkdownTableColumn,
  deleteMarkdownTableRow,
  directiveTarget,
  directiveTone,
  findMarkdownTable,
  insertMarkdownTableColumn,
  insertMarkdownTableRow,
  insertTable,
  markdownTables,
  replaceTextarea,
  rewriteTableDirectiveSource,
  setDirectiveTone,
  tableColumnCount,
  type MarkdownTable,
  type TableDirective,
  type TableEditResult,
} from "./tables.ts";

// Table insertion popover and contextual table editing controls.
function setupTablePalette(toolbar: HTMLElement): void {
  const editor = requiredElement<HTMLTextAreaElement>(
    document,
    "[data-markdown-editor]",
  );
  const open = requiredElement<HTMLButtonElement>(
    toolbar,
    "[data-table-format-open]",
  );
  const insertOwner = requiredElement<HTMLElement>(
    toolbar,
    "[data-table-insert-owner]",
  );
  const insertPopover = requiredElement<HTMLElement>(
    insertOwner,
    "[data-table-insert-popover]",
  );
  const insertGrid = requiredElement<HTMLElement>(
    insertPopover,
    "[data-table-insert-grid]",
  );
  const insertSize = requiredElement<HTMLElement>(
    insertPopover,
    "[data-table-insert-size]",
  );
  const contextToolbar = requiredElement<HTMLElement>(
    document,
    "[data-table-context-toolbar]",
  );
  const contextLabel = requiredElement<HTMLElement>(
    contextToolbar,
    "[data-table-context-label]",
  );
  const actionButtons = requiredElements<HTMLButtonElement>(
    contextToolbar,
    "[data-table-action]",
  );
  const colorTarget = requiredElement<HTMLSelectElement>(
    contextToolbar,
    "[data-table-color-target]",
  );
  const toneButtons = requiredElements<HTMLButtonElement>(
    contextToolbar,
    "[data-table-tone]",
  );
  const sortableControl = requiredElement<HTMLInputElement>(
    contextToolbar,
    "[data-table-format-sortable]",
  );
  const filterableControl = requiredElement<HTMLInputElement>(
    contextToolbar,
    "[data-table-format-filterable]",
  );
  const moreMenu = requiredElement<HTMLDetailsElement>(
    contextToolbar,
    "[data-table-context-more]",
  );
  let currentTable: MarkdownTable | null = null;
  let selectedRows = 3;
  let selectedColumns = 3;

  function syncInsertGrid(rows: number, columns: number): void {
    selectedRows = rows;
    selectedColumns = columns;
    insertSize.textContent = `${rows} × ${columns}`;

    for (const cell of insertGrid.querySelectorAll<HTMLButtonElement>(
      "[data-table-size-cell]",
    )) {
      const cellRows = Number.parseInt(cell.dataset.rows || "0", 10);
      const cellColumns = Number.parseInt(cell.dataset.columns || "0", 10);
      const active = cellRows <= rows && cellColumns <= columns;

      cell.classList.toggle("active", active);
      cell.setAttribute(
        "aria-pressed",
        String(cellRows === rows && cellColumns === columns),
      );
    }
  }

  function buildInsertGrid(): void {
    const fragment = document.createDocumentFragment();

    for (let rows = 1; rows <= 10; rows += 1) {
      for (let columns = 1; columns <= 10; columns += 1) {
        const cell = document.createElement("button");
        cell.type = "button";
        cell.className = "table-size-cell";
        cell.dataset.tableSizeCell = "true";
        cell.dataset.rows = String(rows);
        cell.dataset.columns = String(columns);
        cell.setAttribute("role", "gridcell");
        cell.setAttribute(
          "aria-label",
          `${rows} body rows by ${columns} columns`,
        );
        cell.addEventListener("mouseenter", () =>
          syncInsertGrid(rows, columns),
        );
        cell.addEventListener("focus", () => syncInsertGrid(rows, columns));
        cell.addEventListener("click", () => {
          closeInsertPopover();
          const form = editor.closest<HTMLFormElement>("[data-editor-form]");
          if (form?.dataset.editorMode === "visual") {
            form.dispatchEvent(
              new CustomEvent("editor:visual-table-insert", {
                detail: { rows, columns },
              }),
            );
            return;
          }
          insertTable(editor, rows, columns);
          refreshContext();
        });
        fragment.append(cell);
      }
    }

    insertGrid.replaceChildren(fragment);
    syncInsertGrid(selectedRows, selectedColumns);
  }

  function closeInsertPopover(): void {
    insertPopover.hidden = true;
    open.setAttribute("aria-expanded", "false");
  }

  function openInsertPopover(): void {
    syncInsertGrid(selectedRows, selectedColumns);
    insertPopover.hidden = false;
    open.setAttribute("aria-expanded", "true");
  }

  function selectedDirectiveTarget(): string | null {
    return currentTable
      ? directiveTarget(currentTable, colorTarget.value)
      : null;
  }

  function syncTone(): void {
    const target = selectedDirectiveTarget();
    const tone =
      currentTable && target
        ? directiveTone(currentTable.directive, target)
        : "";

    for (const button of toneButtons) {
      const active = (button.dataset.tableTone ?? "") === tone;
      button.classList.toggle("active", active);
      button.setAttribute("aria-pressed", String(active));
    }
  }

  function syncColorTarget(): void {
    if (!currentTable) return;

    const { kind } = currentTable.context;
    const enabled = new Set<string>(["header"]);
    if (kind === "header" || kind === "body") enabled.add("column");
    if (kind === "body") {
      enabled.add("cell");
      enabled.add("row");
    }

    for (const option of colorTarget.options)
      option.disabled = !enabled.has(option.value);

    if (!enabled.has(colorTarget.value))
      colorTarget.value = kind === "body" ? "cell" : "header";

    syncTone();
  }

  let visualContext: {
    tableIndex: number;
    kind: "header" | "body";
    row: number;
    column: number;
  } | null = null;
  const form = editor.closest<HTMLFormElement>("[data-editor-form]");
  form?.addEventListener("editor:visual-table-context", (event) => {
    const next = (event as CustomEvent).detail;
    if (next && next.kind !== visualContext?.kind)
      colorTarget.value = next.kind === "body" ? "cell" : "header";
    visualContext = next;
    refreshContext();
  });
  form?.addEventListener("editor:mode-change", () => refreshContext());

  function refreshContext(): void {
    if (form?.dataset.editorMode === "visual") {
      currentTable = visualContext
        ? markdownTables(editor.value)[visualContext.tableIndex] || null
        : null;
      if (currentTable && visualContext)
        currentTable.context = { ...visualContext };
    } else
      currentTable = findMarkdownTable(
        editor.value,
        editor.selectionStart ?? 0,
      );
    const kind = currentTable?.context.kind;
    const visible =
      currentTable !== null &&
      (kind === "header" || kind === "separator" || kind === "body");

    contextToolbar.hidden = !visible;
    if (!visible || !currentTable) return;

    const { row, column } = currentTable.context;
    if (kind === "header")
      contextLabel.textContent = `Table · Header · Column ${column}`;
    else if (kind === "body")
      contextLabel.textContent = `Table · Row ${row} · Column ${column}`;
    else contextLabel.textContent = "Table · Separator";

    const lines = editor.value.split("\n");
    const columns = tableColumnCount(lines, currentTable);
    const onBody = kind === "body";
    const onCell = kind === "body" || kind === "header";

    for (const button of actionButtons) {
      const action = button.dataset.tableAction;
      if (action === "insert-row-above" || action === "insert-row-below")
        button.disabled = !onBody;
      else if (action === "delete-row") button.disabled = !onBody;
      else if (
        action === "insert-column-left" ||
        action === "insert-column-right"
      )
        button.disabled = !onCell;
      else if (action === "delete-column")
        button.disabled = !onCell || columns <= 1;
      else if (action === "clear-cell") button.disabled = !onCell;
    }

    sortableControl.checked = currentTable.directive.sortable;
    filterableControl.checked = currentTable.directive.filterable;
    syncColorTarget();
  }

  function writeDirective(nextDirective: TableDirective): void {
    if (!currentTable) return;

    const cursor = editor.selectionStart ?? 0;
    const nextValue = rewriteTableDirectiveSource(
      editor.value,
      currentTable,
      nextDirective,
    );

    replaceTextarea(editor, nextValue, cursor);
    editor.focus();
    refreshContext();
  }

  function applyTableEdit(result: TableEditResult): void {
    replaceTextarea(editor, result.source, result.cursor);
    editor.focus();
    refreshContext();
  }

  open.addEventListener("click", () => {
    refreshContext();
    if (!contextToolbar.hidden) {
      closeInsertPopover();
      return;
    }

    if (insertPopover.hidden) openInsertPopover();
    else closeInsertPopover();
  });

  for (const eventName of ["input", "keyup", "click", "select", "focus"])
    editor.addEventListener(eventName, refreshContext);

  colorTarget.addEventListener("change", syncTone);

  for (const button of toneButtons)
    button.addEventListener("click", () => {
      if (!currentTable || button.disabled) return;

      const target = selectedDirectiveTarget();
      if (!target) return;

      writeDirective(
        setDirectiveTone(
          currentTable.directive,
          target,
          button.dataset.tableTone ?? "",
        ),
      );
    });

  sortableControl.addEventListener("change", () => {
    if (!currentTable || sortableControl.disabled) return;

    const next = cloneDirective(currentTable.directive);
    next.sortable = sortableControl.checked;
    writeDirective(next);
  });

  filterableControl.addEventListener("change", () => {
    if (!currentTable || filterableControl.disabled) return;

    const next = cloneDirective(currentTable.directive);
    next.filterable = filterableControl.checked;
    writeDirective(next);
  });

  for (const button of actionButtons)
    button.addEventListener("click", () => {
      if (!currentTable || button.disabled) return;

      const { row, column } = currentTable.context;
      switch (button.dataset.tableAction) {
        case "insert-row-above":
          applyTableEdit(
            insertMarkdownTableRow(editor.value, currentTable, row, false),
          );
          break;
        case "insert-row-below":
          applyTableEdit(
            insertMarkdownTableRow(editor.value, currentTable, row, true),
          );
          break;
        case "delete-row":
          applyTableEdit(
            deleteMarkdownTableRow(editor.value, currentTable, row),
          );
          break;
        case "insert-column-left":
          applyTableEdit(
            insertMarkdownTableColumn(
              editor.value,
              currentTable,
              column,
              false,
            ),
          );
          break;
        case "insert-column-right":
          applyTableEdit(
            insertMarkdownTableColumn(editor.value, currentTable, column, true),
          );
          break;
        case "delete-column":
          applyTableEdit(
            deleteMarkdownTableColumn(editor.value, currentTable, column),
          );
          break;
        case "clear-cell":
          applyTableEdit(clearMarkdownTableCell(editor.value, currentTable));
          break;
        case "clear-colors": {
          const next = cloneDirective(currentTable.directive);
          next.header = "";
          next.rows = {};
          next.columns = {};
          next.cells = {};
          writeDirective(next);
          moreMenu.open = false;
          break;
        }
        case "delete-table":
          applyTableEdit(deleteMarkdownTable(editor.value, currentTable));
          moreMenu.open = false;
          break;
      }
    });

  document.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Node)) return;

    if (!insertOwner.contains(target)) closeInsertPopover();
    if (!moreMenu.contains(target)) moreMenu.open = false;
  });

  document.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;

    if (!insertPopover.hidden) {
      event.preventDefault();
      closeInsertPopover();
      open.focus();
    }
    moreMenu.open = false;
  });

  insertGrid.addEventListener("mouseleave", () =>
    syncInsertGrid(selectedRows, selectedColumns),
  );

  buildInsertGrid();
  refreshContext();
}

// Initializes table palette.
export function initTablePalette(): void {
  for (const toolbar of document.querySelectorAll<HTMLElement>(
    "[data-markdown-toolbar]",
  ))
    setupTablePalette(toolbar);
}
