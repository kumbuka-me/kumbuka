// Administrator group-member picker behavior.

import {
  createDebouncer,
  createLatestRequest,
  isAbortError,
} from "../../core/async.ts";
import { showNotice } from "../../core/dialogs.ts";
import { requiredAttribute, requiredElement } from "../../core/dom.ts";
import { isRecord, requireArrayOf } from "../../core/guards.ts";
import { errorMessage, requestJSON } from "../../core/http.ts";

interface GroupMember {
  id: number;
  username: string;
  email?: string;
  display_name?: string;
}

function isGroupMember(value: unknown): value is GroupMember {
  return (
    isRecord(value) &&
    typeof value.id === "number" &&
    typeof value.username === "string" &&
    (value.email === undefined || typeof value.email === "string") &&
    (value.display_name === undefined || typeof value.display_name === "string")
  );
}

function groupMembers(value: unknown): GroupMember[] {
  return requireArrayOf(value, isGroupMember, "group member response");
}

// Wires group member picker behavior.
export function setupGroupMemberPicker(card: HTMLElement): void {
  const groupMembersURL = requiredAttribute(card, "data-members-url");
  const userSearchURL = requiredAttribute(card, "data-users-url");
  const personInput = requiredElement<HTMLInputElement>(
    card,
    "[data-group-person-input]",
  );
  const resultList = requiredElement<HTMLElement>(
    card,
    "[data-group-person-results]",
  );
  const renderedMembers = requiredElement<HTMLElement>(
    card,
    "[data-group-member-list]",
  );
  const count = requiredElement<HTMLElement>(card, "[data-group-member-count]");
  let members: GroupMember[] = [];
  const searchRequests = createLatestRequest();
  const searchDebouncer = createDebouncer(() => void searchPeople(), 140);

  // Returns IDs for the currently rendered group members.
  function memberIDs(): Set<number> {
    return new Set(members.map((member) => member.id));
  }

  // Renders members.
  function renderMembers(): void {
    renderedMembers.replaceChildren();

    if (!members.length) {
      const empty = document.createElement("p");

      empty.className = "muted";
      empty.textContent = "No members yet.";
      renderedMembers.append(empty);
    }
    for (const member of members) {
      const chip = document.createElement("div");

      chip.className = "group-member-chip";

      const label = document.createElement("span");
      const name = document.createElement("strong");

      name.textContent = member.display_name || member.username;

      const detail = document.createElement("span");

      detail.textContent = member.email || `@${member.username}`;
      label.append(name, detail);

      const remove = document.createElement("button");

      remove.type = "button";
      remove.title = `Remove ${name.textContent}`;
      remove.setAttribute("aria-label", remove.title);
      remove.textContent = "×";
      remove.addEventListener("click", async () => {
        remove.disabled = true;
        try {
          await requestJSON(`${groupMembersURL}/${member.id}`, {
            method: "DELETE",
          });

          members = members.filter((item) => item.id !== member.id);
          renderMembers();
        } catch (error) {
          console.error("group member removal failed", error);
          await showNotice(
            errorMessage(error) || "Person could not be removed.",
            {
              title: "Member update failed",
            },
          );
          remove.disabled = false;
        }
      });
      chip.append(label, remove);
      renderedMembers.append(chip);
    }
    count.textContent = String(members.length);
  }

  // Loads members.
  async function loadMembers(): Promise<void> {
    try {
      members = groupMembers(await requestJSON(groupMembersURL));
      renderMembers();
    } catch (error) {
      console.error("group members failed", error);
      renderedMembers.innerHTML =
        '<p class="revision-dialog-error">Members could not be loaded.</p>';
    }
  }

  async function addMember(
    user: GroupMember,
    option: HTMLButtonElement,
  ): Promise<void> {
    option.disabled = true;

    try {
      const payload = await requestJSON(groupMembersURL, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: user.id }),
      });
      if (!isGroupMember(payload)) throw new Error("Invalid member response.");

      if (!members.some((member) => member.id === payload.id))
        members.push(payload);

      renderMembers();
      personInput.value = "";
      resultList.hidden = true;
    } catch (error) {
      console.error("group member addition failed", error);
      await showNotice(errorMessage(error) || "Person could not be added.", {
        title: "Member update failed",
      });
      option.disabled = false;
    }
  }

  // Renders results.
  function renderResults(users: GroupMember[]): void {
    resultList.replaceChildren();

    const existing = memberIDs();
    const available = users.filter((user) => !existing.has(user.id));

    if (!available.length) {
      const empty = document.createElement("div");

      empty.className = "muted live-person-result-empty";
      empty.textContent = "No matching people to add.";
      resultList.append(empty);
    }
    for (const user of available) {
      const option = document.createElement("button");

      option.type = "button";
      option.className = "live-person-result";
      option.setAttribute("role", "option");

      const avatar = document.createElement("span");

      avatar.className = "avatar";
      avatar.textContent = (user.display_name || user.username || "?")
        .slice(0, 1)
        .toUpperCase();

      const label = document.createElement("span");
      const name = document.createElement("strong");

      name.textContent = user.display_name || user.username;

      const detail = document.createElement("span");

      detail.textContent = user.email || `@${user.username}`;
      label.append(name, detail);
      option.append(avatar, label);
      option.addEventListener("click", () => void addMember(user, option));
      resultList.append(option);
    }

    resultList.hidden = false;
  }

  // Searches people.
  async function searchPeople(): Promise<void> {
    const query = personInput.value.trim();
    if ([...query].length < 2) {
      searchRequests.abort();
      resultList.hidden = true;
      resultList.replaceChildren();
      return;
    }

    const signal = searchRequests.next();

    try {
      const payload = await requestJSON(
        `${userSearchURL}?q=${encodeURIComponent(query)}`,
        { signal },
      );

      renderResults(groupMembers(payload));
    } catch (error) {
      if (isAbortError(error)) return;

      console.error("person search failed", error);
      resultList.hidden = true;
    }
  }

  personInput.addEventListener("input", () => searchDebouncer.schedule());
  personInput.addEventListener("blur", () =>
    setTimeout(() => {
      resultList.hidden = true;
    }, 120),
  );
  personInput.addEventListener("focus", () => {
    if (
      [...personInput.value.trim()].length >= 2 &&
      resultList.childElementCount
    )
      resultList.hidden = false;
  });

  void loadMembers();
}
