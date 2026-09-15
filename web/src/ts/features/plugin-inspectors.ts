// Generic reading-page inspectors contributed by plugins.

import { requiredElement } from "../core/dom.ts";

export function initPluginInspectors(): void {
  const content = document.querySelector<HTMLElement>(
    "[data-plugin-annotation-content]",
  );
  const tooltip = document.querySelector<HTMLElement>(
    "[data-plugin-inspector-tooltip]",
  );
  if (!content || !tooltip) return;

  for (const panel of document.querySelectorAll<HTMLElement>(
    "[data-plugin-inspector-panel]",
  )) {
    const id = panel.dataset.pluginInspectorPanel;
    if (!id) continue;
    const button = document.querySelector<HTMLButtonElement>(
      `[data-plugin-inspector-open="${CSS.escape(id)}"]`,
    );
    if (button) setupInspector(panel, button, content, tooltip);
  }
}

function setupInspector(
  panel: HTMLElement,
  button: HTMLButtonElement,
  content: HTMLElement,
  tooltip: HTMLElement,
): void {
  const toggle = requiredElement<HTMLInputElement>(
    panel,
    "[data-plugin-inspector-highlight]",
  );
  const close = requiredElement<HTMLButtonElement>(
    panel,
    "[data-plugin-inspector-close]",
  );
  const tooltipName = requiredElement<HTMLElement>(
    tooltip,
    "[data-plugin-inspector-tooltip-name]",
  );
  const tooltipValue = requiredElement<HTMLElement>(
    tooltip,
    "[data-plugin-inspector-tooltip-value]",
  );
  const values = new Map<string, { label: string; value: string }>();

  for (const entry of panel.querySelectorAll<HTMLElement>(
    "[data-plugin-inspector-entry]",
  )) {
    const annotation = entry.dataset.pluginInspectorEntry;
    if (!annotation) continue;
    values.set(annotation, {
      label: entry.dataset.pluginInspectorLabel ?? annotation,
      value:
        requiredElement<HTMLElement>(entry, "[data-plugin-inspector-saved]")
          .textContent ?? "",
    });
  }

  const marks = [
    ...content.querySelectorAll<HTMLElement>("[data-plugin-annotation]"),
  ].filter((mark) => values.has(mark.dataset.pluginAnnotation ?? ""));
  let active: HTMLElement | null = null;

  function hideTooltip(): void {
    active?.removeAttribute("aria-describedby");
    active = null;
    tooltip.hidden = true;
  }

  function showTooltip(mark: HTMLElement): void {
    if (!toggle.checked) return;
    hideTooltip();
    const annotation = mark.dataset.pluginAnnotation ?? "";
    const item = values.get(annotation);
    if (!item) return;
    tooltipName.textContent = item.label;
    tooltipValue.textContent = item.value || "(empty value)";
    tooltip.hidden = false;
    active = mark;
    mark.setAttribute("aria-describedby", tooltip.id);
    const rect = mark.getBoundingClientRect();
    const gap = 8;
    const left = Math.min(
      Math.max(gap, rect.left),
      Math.max(gap, window.innerWidth - tooltip.offsetWidth - gap),
    );
    const below = rect.bottom + gap;
    const top =
      below + tooltip.offsetHeight < window.innerHeight
        ? below
        : Math.max(gap, rect.top - tooltip.offsetHeight - gap);
    tooltip.style.left = `${left}px`;
    tooltip.style.top = `${top}px`;
  }

  function setHighlights(): void {
    button.classList.toggle("active", toggle.checked);
    for (const mark of marks) {
      mark.classList.toggle("plugin-annotation-active", toggle.checked);
      if (toggle.checked) mark.tabIndex = 0;
      else mark.removeAttribute("tabindex");
    }
    hideTooltip();
  }

  function setPanel(open: boolean): void {
    panel.hidden = !open;
    button.setAttribute("aria-expanded", String(open));
    if (open) panel.scrollIntoView({ block: "start" });
  }

  button.addEventListener("click", () => setPanel(Boolean(panel.hidden)));
  close.addEventListener("click", () => {
    setPanel(false);
    button.focus();
  });
  toggle.addEventListener("change", setHighlights);

  for (const mark of marks) {
    mark.addEventListener("pointerenter", (event: PointerEvent) => {
      if (event.pointerType !== "touch") showTooltip(mark);
    });
    mark.addEventListener("pointerleave", () => {
      if (document.activeElement !== mark) hideTooltip();
    });
    mark.addEventListener("focus", () => showTooltip(mark));
    mark.addEventListener("blur", hideTooltip);
    mark.addEventListener("click", (event: MouseEvent) => {
      if (!toggle.checked) return;
      event.preventDefault();
      showTooltip(mark);
    });
  }
  document.addEventListener("keydown", (event: KeyboardEvent) => {
    if (event.key === "Escape") hideTooltip();
  });
  document.addEventListener("pointerdown", (event: PointerEvent) => {
    if (
      event.target instanceof Node &&
      !active?.contains(event.target) &&
      !tooltip.contains(event.target)
    )
      hideTooltip();
  });
  window.addEventListener("scroll", hideTooltip, true);
  window.addEventListener("resize", hideTooltip);
  setHighlights();
}
