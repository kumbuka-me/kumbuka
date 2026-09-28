import { route } from "../core/route.ts";

const identifier = /^[a-z0-9][a-z0-9._-]{0,127}$/;
const digits = /^[0-9]{1,24}$/;
const locale = /^[A-Za-z0-9-]{0,35}$/;

type DeferredReference = {
  plugin: string;
  module: string;
  index: string;
  version: string;
  locale: string;
  slug: string;
};

function reference(block: HTMLElement): DeferredReference | null {
  const plugin = block.dataset.kumbukaDeferredPlugin || "";
  const module = block.dataset.kumbukaDeferredModule || "";
  const index = block.dataset.kumbukaDeferredIndex || "";
  const version = block.dataset.kumbukaDeferredVersion || "";
  const language = block.dataset.kumbukaDeferredLocale || "";
  const slug = document.body?.dataset.currentPage || "";
  if (
    !identifier.test(plugin) ||
    !identifier.test(module) ||
    !digits.test(index) ||
    !digits.test(version) ||
    !locale.test(language) ||
    !slug
  )
    return null;
  return { plugin, module, index, version, locale: language, slug };
}

function fragmentURL(value: DeferredReference): string {
  const slug = value.slug.split("/").map(encodeURIComponent).join("/");
  const path = `/api/plugin-fragments/${encodeURIComponent(value.plugin)}/${encodeURIComponent(value.module)}/${value.index}/${slug}`;
  const parameters = new URLSearchParams({
    v: value.version,
    locale: value.locale,
  });
  return `${route(path)}?${parameters}`;
}

function showFailure(
  block: HTMLElement,
  retry: () => void,
  changed: boolean,
): void {
  block.classList.add("kumbuka-deferred-fragment-error");
  block.setAttribute("role", "alert");
  block.removeAttribute("aria-busy");
  const message = document.createElement("span");
  message.textContent = changed
    ? "This page changed before external content loaded."
    : "External content could not be loaded.";
  const button = document.createElement("button");
  button.type = "button";
  button.className = "button button-secondary button-small";
  button.textContent = changed ? "Reload page" : "Try again";
  button.addEventListener("click", changed ? () => location.reload() : retry, {
    once: true,
  });
  block.replaceChildren(message, button);
}

async function hydrate(block: HTMLElement): Promise<void> {
  if (block.dataset.kumbukaDeferredLoading === "true") return;
  const value = reference(block);
  if (!value) return;
  block.dataset.kumbukaDeferredLoading = "true";
  block.classList.remove("kumbuka-deferred-fragment-error");
  block.setAttribute("aria-busy", "true");
  try {
    const response = await fetch(fragmentURL(value), {
      headers: { Accept: "text/html" },
    });
    if (!response.ok) {
      showFailure(block, () => void hydrate(block), response.status === 409);
      return;
    }
    const template = document.createElement("template");
    template.innerHTML = await response.text();
    block.replaceChildren(template.content);
    block.classList.remove("kumbuka-deferred-fragment");
    block.removeAttribute("role");
    block.removeAttribute("aria-live");
    block.removeAttribute("aria-busy");
    document.dispatchEvent(
      new CustomEvent("kumbuka:fragment-loaded", { detail: block }),
    );
  } catch {
    showFailure(block, () => void hydrate(block), false);
  } finally {
    delete block.dataset.kumbukaDeferredLoading;
  }
}

// hydrateDeferredFragments starts remote content requests without blocking the
// rest of the application bootstrap.
export function hydrateDeferredFragments(root: ParentNode = document): void {
  for (const block of root.querySelectorAll<HTMLElement>(
    "[data-kumbuka-deferred-plugin][data-kumbuka-deferred-module]",
  ))
    void hydrate(block);
}
