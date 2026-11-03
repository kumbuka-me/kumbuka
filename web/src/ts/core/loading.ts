// Shared, host-owned loading indicator. Feature code owns when loading starts and stops.
export type LoadingSize = "small" | "medium" | "large";

// createLoadingIndicator builds an accessible boat and optional progress text.
export function createLoadingIndicator(
  label: string,
  size: LoadingSize = "medium",
): HTMLElement {
  const indicator = document.createElement("div");
  indicator.className = `kumbuka-loading kumbuka-loading--${size}`;
  indicator.setAttribute("role", "status");

  const boat = document.createElement("span");
  boat.className = `kumbuka-boat-spinner kumbuka-boat-spinner--${size}`;
  boat.setAttribute("aria-hidden", "true");
  indicator.append(boat);

  if (label) {
    const message = document.createElement("span");
    message.className = "kumbuka-loading-label";
    message.textContent = label;
    indicator.append(message);
  } else {
    indicator.setAttribute("aria-label", "Loading");
  }

  return indicator;
}

// showDelayedLoading avoids flickering during fast operations and removes only its own indicator.
export function showDelayedLoading(
  container: HTMLElement,
  label: string,
  size: LoadingSize = "medium",
  delay = 250,
): () => void {
  let indicator: HTMLElement | undefined;
  const timer = window.setTimeout(() => {
    indicator = createLoadingIndicator(label, size);
    container.append(indicator);
  }, delay);

  return () => {
    window.clearTimeout(timer);
    indicator?.remove();
  };
}
