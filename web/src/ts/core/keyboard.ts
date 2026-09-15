// Global keyboard shortcuts and shortcut help.

function editableTarget(target: EventTarget | null): boolean {
  return (
    target instanceof HTMLInputElement ||
    target instanceof HTMLTextAreaElement ||
    target instanceof HTMLSelectElement ||
    (target instanceof HTMLElement && target.isContentEditable)
  );
}

// Initializes keyboard workflow.
export function initKeyboardWorkflow(): void {
  const shortcuts = document.querySelector<HTMLDialogElement>(
    "[data-shortcuts-dialog]",
  );
  let pendingG = false;
  let timer: ReturnType<typeof setTimeout> | undefined;

  // Opens shortcuts.
  function openShortcuts(): void {
    if (shortcuts && !shortcuts.open) shortcuts.showModal();
  }
  // Closes shortcuts.
  function closeShortcuts(): void {
    if (shortcuts?.open) shortcuts.close();
  }

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-shortcuts-open]",
  ))
    button.addEventListener("click", openShortcuts);
  for (const button of shortcuts?.querySelectorAll<HTMLButtonElement>(
    "[data-shortcuts-close]",
  ) || [])
    button.addEventListener("click", closeShortcuts);

  shortcuts?.addEventListener("click", (event: MouseEvent) => {
    if (event.target === shortcuts) closeShortcuts();
  });

  function handleKeydown(event: KeyboardEvent): void {
    const modified = event.metaKey || event.ctrlKey || event.altKey;
    if (event.defaultPrevented || modified) return;
    if (event.key === "?" && !editableTarget(event.target)) {
      event.preventDefault();
      openShortcuts();
      return;
    }
    if (editableTarget(event.target) || document.querySelector("dialog[open]"))
      return;

    const key = event.key.toLocaleLowerCase();
    if (pendingG) {
      if (timer !== undefined) clearTimeout(timer);

      pendingG = false;

      if (key === "h") window.location.assign("/");
      else if (key === "g") window.location.assign("/graph");

      return;
    }
    if (key === "g") {
      pendingG = true;
      timer = setTimeout(() => {
        pendingG = false;
      }, 900);
      return;
    }
    if (key === "c" && document.body.dataset.canEdit === "true") {
      event.preventDefault();
      const parent = document.body.dataset.currentPage?.trim();
      window.location.assign(
        parent
          ? `/pages/new?parent=${encodeURIComponent(parent)}`
          : "/pages/new",
      );
      return;
    }

    if (
      key === "e" &&
      document.body.dataset.canEdit === "true" &&
      document.body.dataset.currentPage
    ) {
      event.preventDefault();
      window.location.assign(`/edit/${document.body.dataset.currentPage}`);
    }
  }

  document.addEventListener("keydown", handleKeydown);
}
