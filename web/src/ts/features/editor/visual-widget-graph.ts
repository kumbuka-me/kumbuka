import {
  parseDirectedGraph,
  graphProblem,
  serializeDirectedGraph,
  type DirectedGraphStep,
} from "./directed-graph.ts";

// Edit a detached draft. Only Apply changes the document, so Cancel is lossless.
export function openGraphEditor(
  source: string,
  name: string,
): Promise<string | null> {
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog graph-editor";
  dialog.setAttribute("aria-label", `${name} editor`);
  const previousFocus = document.activeElement;
  const header = document.createElement("div");
  header.className = "app-dialog-header";
  const title = document.createElement("h2");
  title.textContent = `Edit ${name}`;
  const close = button("Close graph editor", () => finish(null));
  header.append(title, close);
  const body = document.createElement("div");
  body.className = "app-dialog-body";
  const help = document.createElement("p");
  help.textContent =
    "Start at the first step. Connect steps and decision branches to later steps. Several branches can share a destination.";
  const layout = document.createElement("div");
  layout.className = "graph-editor-layout";
  const cards = document.createElement("div");
  cards.className = "graph-editor-steps";
  const preview = document.createElement("section");
  preview.className = "graph-editor-preview";
  preview.setAttribute("aria-label", "Graph preview");
  layout.append(cards, preview);
  const error = document.createElement("p");
  error.className = "graph-editor-error";
  error.setAttribute("role", "status");
  const sourceDetails = document.createElement("details");
  const summary = document.createElement("summary");
  summary.textContent = "Graph source";
  const sourceInput = document.createElement("textarea");
  sourceInput.value = source;
  sourceInput.rows = 10;
  sourceInput.setAttribute("aria-label", "Graph source");
  const load = button("Load source into editor", loadSource);
  sourceDetails.append(summary, sourceInput, load);
  const add = button("Add step", () => {
    steps.push(newStep());
    render();
  });
  const footer = document.createElement("div");
  footer.className = "app-dialog-actions";
  const cancel = button("Cancel", () => finish(null));
  const apply = button("Apply graph", () => {
    if (!graphProblem(steps) && !sourceDirty)
      finish(serializeDirectedGraph(steps));
  });
  apply.classList.add("primary");
  footer.append(cancel, apply);
  body.append(help, layout, add, error, sourceDetails);
  dialog.append(header, body, footer);
  document.body.append(dialog);
  let steps: DirectedGraphStep[] = [];
  let sourceDirty = false;
  let resolveResult: (value: string | null) => void;
  const result = new Promise<string | null>((resolve) => {
    resolveResult = resolve;
  });

  function finish(value: string | null): void {
    dialog.close();
    dialog.remove();
    if (previousFocus instanceof HTMLElement && previousFocus.isConnected)
      previousFocus.focus({ preventScroll: true });
    resolveResult(value);
  }

  function newStep(): DirectedGraphStep {
    let number = 1;
    while (steps.some((step) => step.id === `step${number}`)) number++;
    return { id: `step${number}`, title: "New step", routes: [] };
  }

  function updatePreview(): void {
    const problem = graphProblem(steps);
    error.textContent = sourceDirty
      ? "Load your source changes before applying the graph."
      : problem;
    apply.disabled = Boolean(problem) || sourceDirty;
    if (!sourceDirty) sourceInput.value = serializeDirectedGraph(steps);
    for (const option of cards.querySelectorAll<HTMLOptionElement>("option")) {
      const destination = steps.find((step) => step.id === option.value);
      if (destination)
        option.textContent = `${destination.title} (${destination.id})`;
    }
    preview.replaceChildren();
    const heading = document.createElement("h3");
    heading.textContent = "Live preview";
    preview.append(heading);
    const list = document.createElement("ol");
    for (const step of steps) {
      const item = document.createElement("li");
      const name = document.createElement("strong");
      name.textContent = step.title || "Untitled step";
      item.append(name);
      for (const route of step.routes) {
        const line = document.createElement("div");
        line.className = "graph-preview-route";
        line.textContent = `${route.label || "Next"} → ${steps.find((target) => target.id === route.target)?.title || "Choose destination"}`;
        item.append(line);
      }
      if (!step.routes.length) item.append(document.createTextNode(" · End"));
      list.append(item);
    }
    preview.append(list);
  }

  function render(): void {
    cards.replaceChildren();
    for (const [index, step] of steps.entries()) {
      const card = document.createElement("fieldset");
      const legend = document.createElement("legend");
      legend.textContent = `Step ${index + 1} · ${step.id}`;
      const titleInput = document.createElement("input");
      titleInput.value = step.title;
      titleInput.addEventListener("input", () => {
        step.title = titleInput.value;
        updatePreview();
      });
      const kind = document.createElement("select");
      for (const value of ["Step", "Decision", "End"])
        kind.add(new Option(value, value));
      kind.value =
        step.routes.length > 1
          ? "Decision"
          : step.routes.length
            ? "Step"
            : "End";
      kind.addEventListener("change", () => {
        if (kind.value === "End") step.routes = [];
        else if (kind.value === "Step")
          step.routes = [step.routes[0] ?? { label: "", target: "" }];
        else {
          if (!step.routes.length)
            step.routes.push({ label: "Yes", target: "" });
          if (!step.routes[0].label) step.routes[0].label = "Yes";
          if (step.routes.length < 2)
            step.routes.push({ label: "No", target: "" });
        }
        render();
      });
      card.append(legend, field("Title", titleInput), field("Type", kind));
      for (const [routeIndex, route] of step.routes.entries()) {
        const row = document.createElement("div");
        row.className = "graph-editor-route";
        const label = document.createElement("input");
        label.value = route.label;
        label.placeholder =
          step.routes.length > 1 ? "Yes, No, High…" : "Optional";
        label.addEventListener("input", () => {
          route.label = label.value;
          updatePreview();
        });
        const target = document.createElement("select");
        target.add(new Option("Choose destination", ""));
        for (const candidate of steps.slice(index + 1))
          target.add(
            new Option(`${candidate.title} (${candidate.id})`, candidate.id),
          );
        if (
          route.target &&
          !steps
            .slice(index + 1)
            .some((candidate) => candidate.id === route.target)
        ) {
          target.add(
            new Option(`Invalid destination (${route.target})`, route.target),
          );
        }
        target.value = route.target;
        target.addEventListener("change", () => {
          route.target = target.value;
          updatePreview();
        });
        const next = button("Add next step", () => {
          const added = newStep();
          if (
            steps
              .slice(index + 1)
              .some((candidate) => candidate.id === route.target)
          ) {
            added.routes.push({ label: "", target: route.target });
          }
          steps.splice(index + 1, 0, added);
          route.target = added.id;
          render();
        });
        const remove = button("Remove branch", () => {
          step.routes.splice(routeIndex, 1);
          render();
        });
        row.append(
          field("Branch label", label),
          field("Destination", target),
          next,
          remove,
        );
        card.append(row);
      }
      const actions = document.createElement("div");
      actions.className = "graph-editor-actions";
      const branch = button("Add branch", () => {
        step.routes.push({ label: "", target: "" });
        render();
      });
      const up = button("Move up", () => {
        [steps[index - 1], steps[index]] = [steps[index], steps[index - 1]];
        render();
      });
      up.disabled = index === 0;
      const down = button("Move down", () => {
        [steps[index + 1], steps[index]] = [steps[index], steps[index + 1]];
        render();
      });
      down.disabled = index === steps.length - 1;
      const remove = button("Remove step", () => {
        steps.splice(index, 1);
        render();
      });
      actions.append(branch, up, down, remove);
      card.append(actions);
      cards.append(card);
    }
    updatePreview();
  }

  function loadSource(): void {
    try {
      const parsed = parseDirectedGraph(sourceInput.value);
      steps = parsed;
      sourceDirty = false;
      layout.hidden = false;
      add.disabled = false;
      render();
    } catch (problem) {
      error.textContent =
        problem instanceof Error ? problem.message : "Invalid graph source.";
      apply.disabled = true;
      sourceDetails.open = true;
    }
  }
  sourceInput.addEventListener("input", () => {
    sourceDirty = true;
    updatePreview();
  });
  dialog.addEventListener("cancel", (event) => {
    event.preventDefault();
    finish(null);
  });
  layout.hidden = true;
  add.disabled = true;
  loadSource();
  dialog.showModal();
  (cards.querySelector("input") ?? sourceInput).focus();
  return result;
}

function button(label: string, action: () => void): HTMLButtonElement {
  const control = document.createElement("button");
  control.type = "button";
  control.className = "button compact";
  control.textContent = label;
  control.addEventListener("click", action);
  return control;
}

function field(
  label: string,
  input: HTMLInputElement | HTMLSelectElement,
): HTMLLabelElement {
  const wrapper = document.createElement("label");
  const text = document.createElement("span");
  text.textContent = label;
  wrapper.append(text, input);
  return wrapper;
}

// A plugin opts into the graph control through its declarative settings contract.
export function createGraphSetting(
  label: string,
  attribute: string,
  value: string,
): HTMLElement {
  const wrapper = document.createElement("div");
  const source = document.createElement("textarea");
  source.hidden = true;
  source.dataset.widgetAttribute = attribute;
  source.value = value;
  const edit = button(`Edit ${label}`, () => {
    void openGraphEditor(source.value, label).then((next) => {
      if (next === null || !source.isConnected) return;
      source.value = next;
      source.dispatchEvent(new Event("input", { bubbles: true }));
    });
  });
  wrapper.append(source, edit);
  return wrapper;
}
