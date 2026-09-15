// Reusable icon picker behavior.

import { createDebouncer, createLatestRequest, isAbortError } from "./async.ts";
import { requiredElement } from "./dom.ts";
import { isRecord } from "./guards.ts";
import { requestJSON } from "./http.ts";

interface IconOption {
  name: string;
  label: string;
  source: string;
  svg: string;
}

interface IconPage {
  items: IconOption[];
  has_more: boolean;
}

function isIconOption(value: unknown): value is IconOption {
  return (
    isRecord(value) &&
    typeof value.name === "string" &&
    typeof value.label === "string" &&
    typeof value.source === "string" &&
    typeof value.svg === "string"
  );
}

function isIconPage(value: unknown): value is IconPage {
  return (
    isRecord(value) &&
    Array.isArray(value.items) &&
    value.items.every(isIconOption) &&
    typeof value.has_more === "boolean"
  );
}

export function setupIconPicker(dialog: HTMLDialogElement): void {
  const searchInput = requiredElement<HTMLInputElement>(
    dialog,
    "[data-icon-picker-search]",
  );
  const iconGrid = requiredElement<HTMLElement>(
    dialog,
    "[data-icon-picker-grid]",
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-icon-picker-close]",
  );
  const emptyState = requiredElement<HTMLElement>(
    dialog,
    "[data-icon-picker-empty]",
  );

  const iconsURL = dialog.dataset.iconsUrl || "/api/icons";
  const optionRequests = createLatestRequest();
  const searchDebouncer = createDebouncer(
    () => void loadOptions(searchInput.value.trim()),
    120,
  );
  let activeOwner: HTMLElement | null = null;
  let currentQuery = "";
  let nextOffset = 0;
  let hasMore = false;
  let loading = false;

  // Returns the current icon owner value.
  function ownerValue(): string {
    return (
      activeOwner?.querySelector<HTMLInputElement>("[data-icon-picker-value]")
        ?.value || ""
    );
  }

  // Chooses icon.
  function chooseIcon(option: IconOption): void {
    if (!activeOwner) return;

    const { name, label: optionLabel, svg } = option;

    const value = activeOwner.querySelector<HTMLInputElement>(
      "[data-icon-picker-value]",
    );
    const preview = activeOwner.querySelector<HTMLElement>(
      "[data-icon-picker-preview]",
    );
    const label = activeOwner.querySelector<HTMLElement>(
      "[data-icon-picker-label]",
    );

    if (value) {
      value.value = name;
      value.dispatchEvent(new Event("input", { bubbles: true }));
    }
    if (label) label.textContent = optionLabel || "No icon";
    if (preview) {
      preview.replaceChildren();
      if (svg) preview.insertAdjacentHTML("afterbegin", svg);
      else if (preview.dataset.iconPickerEmpty === "plus")
        preview.insertAdjacentHTML(
          "afterbegin",
          '<svg class="lucide-icon" width="20" height="20" viewBox="0 0 24 24" ' +
            'fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" ' +
            'stroke-linejoin="round" aria-hidden="true">' +
            '<path d="M12 5v14M5 12h14"></path></svg>',
        );
    }

    closePicker();
  }

  // Builds one icon-picker option button.
  function optionButton({
    name,
    label,
    source,
    svg,
  }: IconOption): HTMLButtonElement {
    const button = document.createElement("button");

    button.className = "icon-picker-option";
    button.type = "button";
    button.dataset.iconName = name;

    const glyph = document.createElement("span");

    glyph.className = `icon-picker-glyph${name ? "" : " icon-picker-empty"}`;

    if (svg) glyph.insertAdjacentHTML("afterbegin", svg);
    else glyph.textContent = "—";

    const text = document.createElement("span");
    const strong = document.createElement("strong");

    strong.textContent = label;

    const small = document.createElement("small");

    small.textContent = source || "None";
    text.append(strong, small);
    button.append(glyph, text);
    button.addEventListener("click", () =>
      chooseIcon({ name, label, source, svg }),
    );
    return button;
  }

  // Loads options.
  async function loadOptions(
    query: string,
    { append = false }: { append?: boolean } = {},
  ): Promise<void> {
    query = query.trim();
    if (append && (loading || !hasMore || query !== currentQuery)) return;

    if (!append) {
      currentQuery = query;
      nextOffset = 0;
      hasMore = true;
      iconGrid.replaceChildren();
      emptyState.hidden = true;
    }

    const signal = append ? optionRequests.current() : optionRequests.next();
    if (!signal) return;

    const offset = append ? nextOffset : 0;

    loading = true;
    iconGrid.setAttribute("aria-busy", "true");

    try {
      const separator = iconsURL.includes("?") ? "&" : "?";
      const value = await requestJSON(
        `${iconsURL}${separator}q=${encodeURIComponent(query)}&offset=${offset}`,
        { signal },
      );
      if (!isIconPage(value) || signal.aborted) return;

      const options: IconOption[] = [];

      if (
        !append &&
        (!query || "no icon none empty".includes(query.toLocaleLowerCase()))
      ) {
        options.push({
          name: "",
          label: "No icon",
          source: "None",
          svg: "",
        });
      }

      options.push(...value.items);
      iconGrid.append(...options.map(optionButton));
      nextOffset += value.items.length;
      hasMore = value.has_more;

      const selected = ownerValue();

      for (const option of iconGrid.querySelectorAll<HTMLElement>(
        "[data-icon-name]",
      )) {
        option.classList.toggle(
          "selected",
          option.dataset.iconName === selected,
        );
      }

      emptyState.hidden = iconGrid.childElementCount !== 0;
    } catch (error) {
      if (isAbortError(error)) return;
      if (!signal.aborted) hasMore = false;
    } finally {
      if (!signal.aborted) {
        loading = false;
        iconGrid.removeAttribute("aria-busy");
      }
    }
  }

  // Opens picker.
  async function openPicker(owner: HTMLElement): Promise<void> {
    activeOwner = owner;
    searchInput.value = "";
    dialog.showModal();
    await loadOptions("");
    requestAnimationFrame(() => searchInput.focus());
  }

  // Closes picker.
  function closePicker(): void {
    searchDebouncer.cancel();
    optionRequests.abort();
    loading = false;
    iconGrid.removeAttribute("aria-busy");
    dialog.close();
    activeOwner = null;
  }

  document.addEventListener("click", (event: MouseEvent) => {
    const target = event.target;
    if (!(target instanceof Element)) return;

    const button = target.closest<HTMLButtonElement>("[data-icon-picker-open]");
    if (!button) return;

    const owner = button.closest<HTMLElement>("[data-icon-picker-owner]");
    if (owner) void openPicker(owner);
  });

  searchInput.addEventListener("input", () => searchDebouncer.schedule());
  iconGrid.addEventListener("scroll", () => {
    if (
      iconGrid.scrollHeight - iconGrid.scrollTop - iconGrid.clientHeight <
      160
    ) {
      void loadOptions(currentQuery, { append: true });
    }
  });
  close.addEventListener("click", closePicker);
  dialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === dialog) closePicker();
  });
}
