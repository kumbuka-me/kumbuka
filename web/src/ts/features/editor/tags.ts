// Editor tag chips and autocomplete.

import { requiredAttribute, requiredElement } from "../../core/dom.ts";
import { requireArrayOf } from "../../core/guards.ts";
import { requestJSON } from "../../core/http.ts";

// Wires tag editor behavior.
function setupTagEditor(editor: HTMLElement): void {
  const chipList = requiredElement<HTMLElement>(editor, "[data-tag-chips]");
  const tagInput = requiredElement<HTMLInputElement>(
    editor,
    "[data-tag-input]",
  );
  const tagValue = requiredElement<HTMLInputElement>(
    editor,
    "[data-tag-value]",
  );
  const suggestionList = requiredElement<HTMLElement>(
    editor,
    "[data-tag-suggestions]",
  );
  const tagSource = requiredAttribute(editor, "data-tag-source");
  let availableTags: string[] = [];
  let tagsLoaded = false;
  let selectedTags = tagValue.value
    .split(",")
    .map((tag) => tag.trim().toLocaleLowerCase())
    .filter(Boolean);

  // Synchronizes value.
  function syncValue(): void {
    const next = selectedTags.join(",");
    if (tagValue.value === next) return;

    tagValue.value = next;
    tagValue.dispatchEvent(new Event("input", { bubbles: true }));
  }

  // Renders chips.
  function renderChips(): void {
    chipList.replaceChildren();
    for (const tag of selectedTags) {
      const chip = document.createElement("span");

      chip.className = "tag-chip";

      const label = document.createElement("span");

      label.textContent = tag;

      const remove = document.createElement("button");

      remove.type = "button";
      remove.textContent = "×";
      remove.setAttribute("aria-label", `Remove tag ${tag}`);
      remove.addEventListener("click", () => {
        selectedTags = selectedTags.filter((candidate) => candidate !== tag);
        syncValue();
        renderChips();
        renderSuggestions();
        tagInput.focus();
      });

      chip.append(label, remove);
      chipList.append(chip);
    }
  }

  // Adds one normalized tag when it is not already selected.
  function addTag(rawTag: string): void {
    const tag = rawTag.trim().toLocaleLowerCase();
    if (!tag || selectedTags.includes(tag)) return;

    selectedTags.push(tag);
    selectedTags.sort((left, right) => left.localeCompare(right));
    tagInput.value = "";
    syncValue();
    renderChips();
    renderSuggestions();
  }

  // Renders suggestions.
  function renderSuggestions(): void {
    if (!tagsLoaded || document.activeElement !== tagInput) {
      suggestionList.hidden = true;
      return;
    }

    const query = tagInput.value.trim().toLocaleLowerCase();
    const matching = availableTags.filter(
      (tag) =>
        !selectedTags.includes(tag.toLocaleLowerCase()) &&
        tag.toLocaleLowerCase().includes(query),
    );

    suggestionList.replaceChildren();

    for (const tag of matching) {
      const option = document.createElement("button");

      option.type = "button";
      option.className = "tag-suggestion";
      option.setAttribute("role", "option");
      option.textContent = tag;
      option.addEventListener("pointerdown", (event: PointerEvent) => {
        event.preventDefault();
        addTag(tag);
        tagInput.focus();
      });
      suggestionList.append(option);
    }

    suggestionList.hidden = matching.length === 0;
  }

  // Loads tags.
  async function loadTags(): Promise<void> {
    if (tagsLoaded) return;

    try {
      const payload = await requestJSON(tagSource);

      availableTags = requireArrayOf(
        payload,
        (tag): tag is string => typeof tag === "string",
        "tag response",
      );
      tagsLoaded = true;
      renderSuggestions();
    } catch (error) {
      console.error("tag autocomplete failed", error);
    }
  }

  tagInput.addEventListener("focus", async () => {
    await loadTags();
    renderSuggestions();
  });
  tagInput.addEventListener("input", renderSuggestions);
  tagInput.addEventListener("keydown", (event: KeyboardEvent) => {
    switch (event.key) {
      case "Enter":
      case ",":
        if (tagInput.value.trim()) {
          event.preventDefault();
          addTag(tagInput.value);
        }
        break;
      case "Backspace":
        if (!tagInput.value && selectedTags.length) {
          selectedTags.pop();
          syncValue();
          renderChips();
          renderSuggestions();
        }
        break;
      case "Escape":
        suggestionList.hidden = true;
        break;
    }
  });
  tagInput.addEventListener("blur", () => {
    if (tagInput.value.trim()) addTag(tagInput.value);
    setTimeout(() => (suggestionList.hidden = true), 0);
  });

  const form = editor.closest<HTMLFormElement>("form");

  form?.addEventListener("submit", () => {
    if (tagInput.value.trim()) addTag(tagInput.value);
    syncValue();
  });
  form?.addEventListener("editor:restore-draft", (event) => {
    const raw = String(event.detail.values.tags?.[0] || "");

    selectedTags = raw
      .split(",")
      .map((tag) => tag.trim().toLocaleLowerCase())
      .filter(Boolean);
    tagInput.value = "";
    tagValue.value = selectedTags.join(",");
    renderChips();
    renderSuggestions();
  });

  renderChips();
  syncValue();
}

// Initializes tags.
export function initTags(): void {
  for (const editor of document.querySelectorAll<HTMLElement>(
    "[data-tag-editor]",
  ))
    setupTagEditor(editor);
}
