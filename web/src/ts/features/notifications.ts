// Notification inbox interactions.

import { showNotice } from "../core/dialogs.ts";
import { requestJSON } from "../core/http.ts";

async function markRead(id: string): Promise<void> {
  await requestJSON(`/api/notifications/${id}/read`, {
    method: "POST",
    keepalive: true,
  });
}

async function readAllNotifications(
  menu: HTMLElement,
  trigger: HTMLButtonElement,
): Promise<void> {
  trigger.disabled = true;

  try {
    await markRead("all");
  } catch {
    trigger.disabled = false;
    await showNotice("Notifications could not be marked as read. Try again.", {
      title: "Notifications",
    });
    return;
  }

  for (const item of menu.querySelectorAll<HTMLElement>(
    ".notification-item.unread",
  )) {
    item.classList.remove("unread");
  }

  menu.querySelector<HTMLElement>(".notification-badge")?.remove();
  trigger.remove();
}

function handleReadAllClick(menu: HTMLElement, event: MouseEvent): void {
  event.preventDefault();

  const trigger = event.currentTarget;
  if (!(trigger instanceof HTMLButtonElement)) return;

  void readAllNotifications(menu, trigger);
}

// initNotifications initializes the notification inbox controls.
export function initNotifications(): void {
  const menu = document.querySelector<HTMLElement>("[data-notification-menu]");
  if (!menu) return;

  menu
    .querySelector<HTMLButtonElement>("[data-notifications-read-all]")
    ?.addEventListener("click", (event: MouseEvent) =>
      handleReadAllClick(menu, event),
    );
}
