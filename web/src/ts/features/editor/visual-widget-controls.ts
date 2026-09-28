// Shared scalar controls for plugin widget forms.

import {
  mentionReplacement,
  renderMentionSuggestions,
  searchMentionUsers,
  type MentionUser,
} from "../mentions.ts";

let widgetMentionSequence = 0;

export function createLabel(text: string): HTMLLabelElement {
  const label = document.createElement("label");
  label.className = "visual-widget-field";
  const title = document.createElement("span");
  title.className = "visual-widget-field-label";
  title.textContent = text;
  label.append(title);
  return label;
}

// setupMentionSetting adds bounded user autocomplete and canonical mention selection.
export function setupMentionSetting(
  input: HTMLInputElement,
  wrapper: HTMLElement,
): void {
  wrapper.classList.add("mention-suggestion-anchor");
  const menu = document.createElement("div");
  menu.id = `visual-widget-mentions-${++widgetMentionSequence}`;
  menu.className = "mention-suggestion-menu visual-widget-mention-menu";
  menu.hidden = true;
  menu.setAttribute("role", "listbox");
  menu.setAttribute("aria-label", "Mention a user");
  input.autocomplete = "off";
  input.pattern = "@[A-Za-z0-9_.-]+";
  input.setAttribute("aria-expanded", "false");
  wrapper.append(menu);
  let results: MentionUser[] = [];
  let active = -1;
  let request = 0;

  const close = () => {
    request += 1;
    results = [];
    active = -1;
    menu.hidden = true;
    menu.replaceChildren();
    input.removeAttribute("aria-controls");
    input.setAttribute("aria-expanded", "false");
  };
  const render = (query: string) => {
    renderMentionSuggestions(menu, results, query, active);
    menu.hidden = false;
    input.setAttribute("aria-controls", menu.id);
    input.setAttribute("aria-expanded", "true");
  };
  const choose = (index: number) => {
    const user = results[index];
    if (!user) return;
    input.value = mentionReplacement(user.username);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    close();
    input.focus();
  };
  const canonicalize = () => {
    const current = input.value.trim().toLocaleLowerCase();
    const match = results.find(
      (user) => `@${user.username}`.toLocaleLowerCase() === current,
    );
    if (match) input.value = mentionReplacement(match.username);
  };
  input.addEventListener("change", canonicalize);
  input.addEventListener("input", () => {
    const value = input.value.trim();
    if (!value.startsWith("@")) {
      close();
      return;
    }
    const current = ++request;
    const query = value.slice(1);
    void searchMentionUsers(query)
      .then((users) => {
        if (current !== request) return;
        results = users.slice(0, 50);
        active = results.length ? 0 : -1;
        render(query);
      })
      .catch((error) => {
        console.error("widget mention search failed", error);
        if (current === request) close();
      });
  });
  input.addEventListener("keydown", (event) => {
    if (menu.hidden) return;
    if (event.key === "ArrowDown" && results.length) {
      event.preventDefault();
      active = (active + 1) % results.length;
      render(input.value.trim().slice(1));
    } else if (event.key === "ArrowUp" && results.length) {
      event.preventDefault();
      active = (active - 1 + results.length) % results.length;
      render(input.value.trim().slice(1));
    } else if ((event.key === "Enter" || event.key === "Tab") && active >= 0) {
      event.preventDefault();
      choose(active);
    } else if (event.key === "Escape") {
      event.preventDefault();
      close();
    }
  });
  menu.addEventListener("mousedown", (event) => event.preventDefault());
  menu.addEventListener("click", (event) => {
    const target = event.target;
    if (!(target instanceof Element)) return;
    const option = target.closest<HTMLElement>("[data-mention-index]");
    if (option) choose(Number(option.dataset.mentionIndex));
  });
  input.addEventListener("blur", () => window.setTimeout(close));
}
