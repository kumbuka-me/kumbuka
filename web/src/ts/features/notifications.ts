// Notification inbox interactions.

import { showNotice } from "../core/dialogs.ts";
import { requestJSON } from "../core/http.ts";

async function setReadState(id: string, read: boolean): Promise<void> {
  await requestJSON(`/api/notifications/${id}/${read ? "read" : "unread"}`, {
    method: "POST",
    keepalive: true,
  });
}

async function removeNotification(id: string): Promise<void> {
  await requestJSON(`/api/notifications/${id}`, {
    method: "DELETE",
    keepalive: true,
  });
}

function rows(menu: HTMLElement): HTMLElement[] {
  return [
    ...menu.querySelectorAll<HTMLElement>(
      "[data-notification-list] [data-notification-id]",
    ),
  ];
}

function unreadCount(menu: HTMLElement): number {
  const badge = menu.querySelector<HTMLElement>("[data-notification-badge]");
  if (!badge) return 0;

  const value = Number.parseInt(badge.textContent ?? "0", 10);
  return Number.isFinite(value) && value > 0 ? value : 0;
}

function setUnreadCount(menu: HTMLElement, count: number): void {
  const unread = Math.max(0, count);
  const badge = menu.querySelector<HTMLElement>("[data-notification-badge]");
  const readAll = menu.querySelector<HTMLButtonElement>(
    "[data-notifications-read-all]",
  );

  if (badge) {
    badge.textContent = String(unread);
    badge.hidden = unread === 0;
  }
  if (readAll) readAll.hidden = unread === 0;
}

function syncEmptyState(menu: HTMLElement): void {
  const list = menu.querySelector<HTMLElement>("[data-notification-list]");
  if (!list) return;

  const items = rows(menu);
  const empty = list.querySelector<HTMLElement>(".notification-empty");
  if (items.length === 0 && !empty) {
    const message = document.createElement("p");
    message.className = "muted notification-empty";
    message.textContent = "Nothing new.";
    list.append(message);
  } else if (items.length > 0) {
    empty?.remove();
  }
}

function setRowReadState(row: HTMLElement, read: boolean): void {
  row.classList.toggle("unread", !read);

  const trigger = row.querySelector<HTMLButtonElement>(
    "[data-notification-toggle-read]",
  );
  if (!trigger) return;

  trigger.textContent = read ? "Unread" : "Read";
  trigger.setAttribute(
    "aria-label",
    read ? "Mark notification unread" : "Mark notification read",
  );
}

async function readAllNotifications(
  menu: HTMLElement,
  trigger: HTMLButtonElement,
): Promise<void> {
  trigger.disabled = true;

  try {
    await setReadState("all", true);
  } catch {
    trigger.disabled = false;
    await showNotice("Notifications could not be marked as read. Try again.", {
      title: "Notifications",
    });
    return;
  }

  for (const item of rows(menu)) setRowReadState(item, true);
  setUnreadCount(menu, 0);
  trigger.disabled = false;
}

async function toggleNotification(
  menu: HTMLElement,
  row: HTMLElement,
  trigger: HTMLButtonElement,
): Promise<void> {
  const id = row.dataset.notificationId;
  if (!id) return;

  const wasUnread = row.classList.contains("unread");
  const read = wasUnread;
  trigger.disabled = true;

  try {
    await setReadState(id, read);
    setRowReadState(row, read);
    setUnreadCount(menu, unreadCount(menu) + (read ? -1 : 1));
  } catch {
    await showNotice(
      `The notification could not be marked as ${read ? "read" : "unread"}. Try again.`,
      { title: "Notifications" },
    );
  } finally {
    trigger.disabled = false;
  }
}

async function deleteNotification(
  menu: HTMLElement,
  row: HTMLElement,
  trigger: HTMLButtonElement,
): Promise<void> {
  const id = row.dataset.notificationId;
  if (!id) return;

  const wasUnread = row.classList.contains("unread");
  trigger.disabled = true;

  try {
    await removeNotification(id);
    row.remove();
    if (wasUnread) setUnreadCount(menu, unreadCount(menu) - 1);
    syncEmptyState(menu);
  } catch {
    trigger.disabled = false;
    await showNotice("The notification could not be deleted. Try again.", {
      title: "Notifications",
    });
  }
}

function handleReadAllClick(menu: HTMLElement, event: MouseEvent): void {
  event.preventDefault();

  const trigger = event.currentTarget;
  if (!(trigger instanceof HTMLButtonElement)) return;

  void readAllNotifications(menu, trigger);
}

function setupNotificationRow(menu: HTMLElement, row: HTMLElement): void {
  row
    .querySelector<HTMLButtonElement>("[data-notification-toggle-read]")
    ?.addEventListener("click", (event: MouseEvent) => {
      event.preventDefault();
      const trigger = event.currentTarget;
      if (!(trigger instanceof HTMLButtonElement)) return;

      void toggleNotification(menu, row, trigger);
    });

  row
    .querySelector<HTMLButtonElement>("[data-notification-delete]")
    ?.addEventListener("click", (event: MouseEvent) => {
      event.preventDefault();
      const trigger = event.currentTarget;
      if (!(trigger instanceof HTMLButtonElement)) return;

      void deleteNotification(menu, row, trigger);
    });
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

  for (const row of rows(menu)) setupNotificationRow(menu, row);
  setUnreadCount(menu, unreadCount(menu));
  syncEmptyState(menu);
}
