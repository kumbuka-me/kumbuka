import { t } from "./i18n.ts";

// Clipboard helpers shared by interactive controls.

export async function copyText(text: string): Promise<void> {
  if (!navigator.clipboard?.writeText) {
    throw new Error(
      t(
        "browser.clipboard.unavailable",
        "Clipboard API is not available in this browser or context.",
      ),
    );
  }

  await navigator.clipboard.writeText(text);
}

// Wires the standard Copy/Copied interaction used by copyable values.
export function setupCopyButton(
  button: HTMLButtonElement,
  value: () => string,
  ariaLabel: string,
): void {
  button.textContent = t("browser.clipboard.copy", "Copy");
  button.setAttribute("aria-label", ariaLabel);

  let resetTimer: ReturnType<typeof setTimeout> | undefined;

  button.addEventListener("click", async () => {
    if (resetTimer) clearTimeout(resetTimer);

    button.disabled = true;

    try {
      await copyText(value());
      button.textContent = t("browser.clipboard.copied", "Copied");
      button.classList.add("copied");
    } catch (error) {
      console.error("copy to clipboard failed", error);
      button.textContent = t("browser.clipboard.failed", "Copy failed");
    } finally {
      button.disabled = false;
      resetTimer = setTimeout(() => {
        button.textContent = t("browser.clipboard.copy", "Copy");
        button.classList.remove("copied");
      }, 1600);
    }
  });
}
