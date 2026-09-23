// Searchable custom dialog for choosing fenced-code languages.

import {
  canonicalCodeLanguage,
  filterCodeLanguages,
} from "./code-languages.ts";

// openCodeLanguagePicker opens the visual code-language chooser.
export function openCodeLanguagePicker(
  current: string,
): Promise<string | null> {
  const dialog = document.createElement("dialog");
  dialog.className = "app-dialog code-language-dialog";
  dialog.setAttribute("aria-label", "Code block language");

  const header = document.createElement("div");
  header.className = "app-dialog-header";
  const heading = document.createElement("div");
  const eyebrow = document.createElement("p");
  eyebrow.className = "eyebrow";
  eyebrow.textContent = "Code block";
  const title = document.createElement("h2");
  title.textContent = "Choose language";
  heading.append(eyebrow, title);
  const close = document.createElement("button");
  close.type = "button";
  close.className = "icon-button app-dialog-close";
  close.setAttribute("aria-label", "Close language picker");
  close.textContent = "×";
  header.append(heading, close);

  const search = document.createElement("input");
  search.type = "search";
  search.className = "code-language-search";
  search.placeholder = "Search languages…";
  search.autocomplete = "off";
  search.setAttribute("aria-label", "Search code languages");

  const list = document.createElement("div");
  list.className = "code-language-list";
  list.setAttribute("role", "listbox");
  list.setAttribute("aria-label", "Supported languages");

  const empty = document.createElement("p");
  empty.className = "code-language-empty muted";
  empty.textContent = "No matching supported languages.";
  empty.hidden = true;

  const body = document.createElement("div");
  body.className = "app-dialog-body code-language-body";
  body.append(search, list, empty);
  dialog.append(header, body);
  document.body.append(dialog);

  return new Promise<string | null>((resolve) => {
    let settled = false;

    // finish closes the picker exactly once and resolves the selected fence value.
    const finish = (value: string | null): void => {
      if (settled) return;
      settled = true;
      if (dialog.open) dialog.close();
      dialog.remove();
      resolve(value);
    };

    const render = (): void => {
      const currentLanguage = canonicalCodeLanguage(current);
      const languages = filterCodeLanguages(search.value);
      list.replaceChildren();

      for (const language of languages) {
        const option = document.createElement("button");
        option.type = "button";
        option.className = "code-language-option";
        option.setAttribute("role", "option");
        option.setAttribute(
          "aria-selected",
          String(language.value === currentLanguage),
        );

        const label = document.createElement("strong");
        label.textContent = language.label;
        option.append(label);
        option.addEventListener("click", () => finish(language.value));
        list.append(option);
      }

      empty.hidden = languages.length > 0;
    };

    search.addEventListener("input", render);
    close.addEventListener("click", () => finish(null));
    dialog.addEventListener("cancel", (event) => {
      event.preventDefault();
      finish(null);
    });
    dialog.addEventListener("click", (event) => {
      if (event.target === dialog) finish(null);
    });

    render();
    dialog.showModal();
    search.focus();
  });
}
