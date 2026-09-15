// The single breadcrumb/autocomplete picker for all page-path fields.
export function initPathPickers(): void {
  const paths = new Map<string, string>();
  document
    .querySelectorAll<HTMLElement>("[data-path-option]")
    .forEach((item) => {
      const slug = item.dataset.slug || "";
      const parts = slug.split("/").filter(Boolean);
      parts.forEach((part, index) => {
        const path = parts.slice(0, index + 1).join("/");
        if (!paths.has(path)) paths.set(path, part);
      });
      if (slug) paths.set(slug, item.dataset.label || slug);
    });

  document
    .querySelectorAll<HTMLInputElement>("[data-path-input]")
    .forEach((input, pickerIndex) => {
      const wrapper = document.createElement("div");
      wrapper.className = "shared-path-picker";
      input.before(wrapper);
      // Keep the input in its original label, and preserve its submitted value.
      wrapper.append(input);
      const crumbs = document.createElement("div");
      crumbs.className = "shared-path-crumbs";
      crumbs.setAttribute("aria-label", "Selected path");
      input.before(crumbs);
      const suggestions = document.createElement("div");
      suggestions.className = "shared-path-suggestions";
      suggestions.setAttribute("aria-label", "Choose a path");
      wrapper.append(suggestions);
      suggestions.id = `path-suggestions-${pickerIndex}`;
      suggestions.setAttribute("role", "listbox");
      input.setAttribute("role", "combobox");
      input.setAttribute("aria-autocomplete", "list");
      input.setAttribute("aria-controls", suggestions.id);
      let active = 0;
      function close(): void {
        suggestions.hidden = true;
        input.setAttribute("aria-expanded", "false");
        input.removeAttribute("aria-activedescendant");
      }
      function highlight(): void {
        const options = [
          ...suggestions.querySelectorAll<HTMLButtonElement>("[role=option]"),
        ];
        options.forEach((option, index) => {
          option.classList.toggle("active", index === active);
          option.setAttribute("aria-selected", String(index === active));
        });
        input.setAttribute("aria-expanded", String(!suggestions.hidden));
        if (!suggestions.hidden && options[active])
          input.setAttribute("aria-activedescendant", options[active].id);
        else input.removeAttribute("aria-activedescendant");
      }

      function choose(path: string): void {
        active = 0;
        input.value = path;
        input.dispatchEvent(new Event("input", { bubbles: true }));
        input.dispatchEvent(new Event("change", { bubbles: true }));
        input.focus();
      }
      function button(
        label: string,
        path: string,
        className: string,
      ): HTMLButtonElement {
        const item = document.createElement("button");
        item.type = "button";
        item.className = className;
        item.textContent = label;
        item.title = path || "Top level";
        // Keep focus on the input so blur/change cannot replace the clicked button.
        item.addEventListener("pointerdown", (event) => event.preventDefault());
        item.addEventListener("click", (event) => {
          event.preventDefault();
          choose(path);
        });
        return item;
      }
      function render(): void {
        wrapper.hidden = input.hidden;
        crumbs.replaceChildren(button("Top level", "", "path-crumb"));
        const value = input.value.trim().replace(/^\/+|\/+$/g, "");
        const parts = value.split("/").filter(Boolean);
        parts.forEach((part, index) => {
          const separator = document.createElement("span");
          separator.textContent = "/";
          separator.setAttribute("aria-hidden", "true");
          crumbs.append(
            separator,
            button(part, parts.slice(0, index + 1).join("/"), "path-crumb"),
          );
        });
        suggestions.replaceChildren();
        const matches = [...paths]
          .filter(([path, label]) => {
            if (!value) return !path.includes("/");
            if (path.startsWith(value + "/"))
              return !path.slice(value.length + 1).includes("/");
            return (
              path !== value &&
              (path.toLowerCase().includes(value.toLowerCase()) ||
                label.toLowerCase().includes(value.toLowerCase()))
            );
          })
          .slice(0, 20);
        active = Math.min(active, Math.max(0, matches.length - 1));
        matches.forEach(([path, label], index) => {
          const option = button(label, path, "path-suggestion");
          option.id = `${suggestions.id}-${index}`;
          option.setAttribute("role", "option");
          option.tabIndex = -1;
          const detail = document.createElement("small");
          detail.textContent = path;
          option.append(detail);
          suggestions.append(option);
        });
        suggestions.hidden =
          matches.length === 0 || document.activeElement !== input;
        highlight();
      }
      input.autocomplete = "off";
      input.addEventListener("input", () => {
        active = 0;
        render();
      });
      input.addEventListener("change", render);
      input.addEventListener("focus", render);
      input.addEventListener("blur", close);
      input.addEventListener("keydown", (event) => {
        if (event.isComposing) return;
        if (event.key === "Escape") {
          close();
          event.preventDefault();
          return;
        }
        const options = [
          ...suggestions.querySelectorAll<HTMLButtonElement>("[role=option]"),
        ];
        if (event.key === "Enter" && !suggestions.hidden && options[active]) {
          event.preventDefault();
          options[active].click();
        }
        if (
          (event.key === "ArrowDown" || event.key === "ArrowUp") &&
          options.length
        ) {
          event.preventDefault();
          active = suggestions.hidden
            ? 0
            : (active + (event.key === "ArrowDown" ? 1 : options.length - 1)) %
              options.length;
          suggestions.hidden = false;
          highlight();
          options[active]?.scrollIntoView({ block: "nearest" });
        }
      });
      input.form?.addEventListener("editor:restore-draft", () =>
        requestAnimationFrame(render),
      );
      input.form?.addEventListener("reset", () => setTimeout(render, 0));
      new MutationObserver(render).observe(input, {
        attributes: true,
        attributeFilter: ["hidden"],
      });
      render();
    });
}
