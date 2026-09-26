// Administrator webhook payload, header, and retry configuration behavior.

import { showNotice, showProblemDialog } from "../../core/dialogs.ts";
import { requiredElement } from "../../core/dom.ts";
import {
  errorMessage,
  parseProblemPayload,
  responseProblem,
  type ProblemPayload,
} from "../../core/http.ts";

const sensitiveWebhookHeaderNames = new Set([
  "authorization",
  "cookie",
  "proxy-authorization",
  "x-api-key",
  "x-auth-token",
]);

type WebhookTemplateSuggestion = {
  value: string;
  description: string;
};

export type WebhookTemplateCompletions = {
  start: number;
  end: number;
  items: WebhookTemplateSuggestion[];
};

const webhookTemplateSuggestions: WebhookTemplateSuggestion[] = [
  { value: ".Input.Event", description: "Event name" },
  { value: ".Payload.ActorID", description: "Actor's stable user ID" },
  { value: ".Payload.ObjectType", description: "Affected resource type" },
  { value: ".Payload.ObjectKey", description: "Affected resource key" },
  { value: ".Payload.Detail", description: "Human-readable event detail" },
  { value: ".Payload.Data", description: "Event-specific values" },
  { value: ".Payload.OccurredAt", description: "UTC event timestamp" },
  { value: ".Payload.URL", description: "Public page URL" },
  { value: ".Payload.Actor.ID", description: "Actor's stable user ID" },
  { value: ".Payload.Actor.Mention", description: "Actor's @mention" },
  { value: ".Payload.Actor.DisplayName", description: "Actor's display name" },
  { value: ".Payload.Actor.Email", description: "Actor's email address" },
  { value: ".Payload.Actor.Enabled", description: "Actor account status" },
  { value: ".Payload.Recipient.ID", description: "Recipient's stable user ID" },
  { value: ".Payload.Recipient.Mention", description: "Recipient's @mention" },
  {
    value: ".Payload.Recipient.DisplayName",
    description: "Recipient's display name",
  },
  {
    value: ".Payload.Recipient.Email",
    description: "Recipient's email address",
  },
  {
    value: ".Payload.Recipient.Enabled",
    description: "Recipient account status",
  },
];

// Returns field suggestions for the template token immediately before the caret.
export function webhookTemplateCompletions(
  value: string,
  caret: number,
): WebhookTemplateCompletions | null {
  const beforeCaret = value.slice(0, caret);
  const token = beforeCaret.match(/\.[A-Za-z0-9_.]*$/)?.[0];
  if (!token || !/^\.(?:i|p)/i.test(token)) return null;

  const normalized = token.toLowerCase();
  const items = webhookTemplateSuggestions.filter((item) =>
    item.value.toLowerCase().startsWith(normalized),
  );
  if (items.length === 0) return null;

  return { start: caret - token.length, end: caret, items };
}

function setupWebhookHelp(): void {
  const dialog = document.querySelector<HTMLDialogElement>(
    "[data-webhook-help-dialog]",
  );
  if (!dialog) return;

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-webhook-help-open]",
  )) {
    button.addEventListener("click", () => {
      if (!dialog.open) dialog.showModal();
    });
  }
  for (const button of dialog.querySelectorAll<HTMLButtonElement>(
    "[data-webhook-help-close]",
  )) {
    button.addEventListener("click", () => dialog.close());
  }
}

function setupWebhookTemplateAutocomplete(textarea: HTMLTextAreaElement): void {
  const popup = document.createElement("div");
  popup.className = "webhook-template-completions";
  popup.hidden = true;
  popup.setAttribute("role", "listbox");
  document.body.append(popup);

  let completion: WebhookTemplateCompletions | null = null;
  let activeIndex = 0;

  const hide = (): void => {
    popup.hidden = true;
    completion = null;
    textarea.removeAttribute("aria-activedescendant");
  };

  const accept = (index: number): void => {
    const item = completion?.items[index];
    if (!completion || !item) return;
    textarea.setRangeText(item.value, completion.start, completion.end, "end");
    textarea.dispatchEvent(new Event("input", { bubbles: true }));
    textarea.focus();
    hide();
  };

  const render = (): void => {
    completion = webhookTemplateCompletions(
      textarea.value,
      textarea.selectionStart,
    );
    popup.replaceChildren();
    if (!completion) {
      hide();
      return;
    }

    activeIndex = Math.min(activeIndex, completion.items.length - 1);
    completion.items.forEach((item, index) => {
      const option = document.createElement("button");
      option.type = "button";
      option.className = "webhook-template-completion";
      option.dataset.active = String(index === activeIndex);
      option.setAttribute("role", "option");
      option.setAttribute("aria-selected", String(index === activeIndex));
      const field = document.createElement("code");
      field.textContent = item.value;
      const description = document.createElement("span");
      description.textContent = item.description;
      option.append(field, description);
      option.addEventListener("pointerdown", (event) => event.preventDefault());
      option.addEventListener("click", () => accept(index));
      popup.append(option);
    });

    const bounds = textarea.getBoundingClientRect();
    popup.style.left = `${bounds.left}px`;
    popup.style.top = `${bounds.bottom + 4}px`;
    popup.style.width = `${bounds.width}px`;
    popup.hidden = false;
  };

  textarea.addEventListener("input", () => {
    activeIndex = 0;
    render();
  });
  textarea.addEventListener("click", render);
  textarea.addEventListener("blur", () => window.setTimeout(hide));
  textarea.addEventListener("keydown", (event) => {
    if (popup.hidden || !completion) return;
    if (event.key === "Escape") {
      event.preventDefault();
      hide();
      return;
    }
    if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const direction = event.key === "ArrowDown" ? 1 : -1;
      activeIndex =
        (activeIndex + direction + completion.items.length) %
        completion.items.length;
      render();
      return;
    }
    if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      accept(activeIndex);
    }
  });
}

function setWebhookHeaderStatus(
  form: HTMLFormElement,
  state: "" | "success" | "error",
  message: string,
): void {
  const status = requiredElement<HTMLElement>(
    form,
    "[data-webhook-header-status]",
  );
  status.dataset.state = state;
  status.textContent = message;
}

function assignWebhookHeaderRowNames(row: HTMLElement, token: string): void {
  const prefix = `webhook_header_${token}_`;
  const rowKey = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-row-key]",
  );
  const id = requiredElement<HTMLInputElement>(row, "[data-webhook-header-id]");
  const name = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-name]",
  );
  const value = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-value]",
  );
  const sensitive = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-sensitive]",
  );

  rowKey.name = "webhook_header_row";
  rowKey.value = token;
  id.name = `${prefix}id`;
  name.name = `${prefix}name`;
  value.name = `${prefix}value`;
  sensitive.name = `${prefix}sensitive`;
}

async function revealWebhookHeader(
  form: HTMLFormElement,
  row: HTMLElement,
  button: HTMLButtonElement,
): Promise<void> {
  const webhookID = button.dataset.webhookId;
  const headerID = button.dataset.webhookHeaderId;
  if (!webhookID || !headerID) return;

  button.disabled = true;
  setWebhookHeaderStatus(form, "", "");

  try {
    const response = await fetch(
      `/admin/webhooks/${encodeURIComponent(webhookID)}/headers/${encodeURIComponent(headerID)}/reveal`,
      {
        method: "POST",
        credentials: "same-origin",
        headers: { Accept: "application/json" },
      },
    );
    if (!response.ok) throw await responseProblem(response);

    const payload = (await response.json()) as { value?: unknown };
    if (typeof payload.value !== "string") {
      throw new Error("The server returned an invalid webhook header value.");
    }

    const value = requiredElement<HTMLInputElement>(
      row,
      "[data-webhook-header-value]",
    );
    value.type = "text";
    value.value = payload.value;
    value.focus();
    button.textContent = "Revealed";
    setWebhookHeaderStatus(form, "success", "Sensitive header revealed.");
  } catch (error: unknown) {
    button.disabled = false;
    setWebhookHeaderStatus(form, "error", errorMessage(error));
  }
}

function bindWebhookHeaderRow(form: HTMLFormElement, row: HTMLElement): void {
  const name = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-name]",
  );
  const value = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-value]",
  );
  const sensitive = requiredElement<HTMLInputElement>(
    row,
    "[data-webhook-header-sensitive]",
  );
  const remove = requiredElement<HTMLButtonElement>(
    row,
    "[data-webhook-header-remove]",
  );
  const reveal = row.querySelector<HTMLButtonElement>(
    "[data-webhook-header-reveal]",
  );

  const refreshSensitiveValue = (): void => {
    if (!reveal || reveal.textContent !== "Revealed") {
      value.type = sensitive.checked ? "password" : "text";
    }
  };

  sensitive.addEventListener("change", () => {
    sensitive.dataset.userSet = "true";
    refreshSensitiveValue();
    setWebhookHeaderStatus(form, "", "");
  });

  name.addEventListener("input", () => {
    if (
      row.dataset.webhookHeaderExisting !== "true" &&
      sensitive.dataset.userSet !== "true"
    ) {
      sensitive.checked = sensitiveWebhookHeaderNames.has(
        name.value.trim().toLowerCase(),
      );
      refreshSensitiveValue();
    }
    setWebhookHeaderStatus(form, "", "");
  });

  value.addEventListener("input", () => setWebhookHeaderStatus(form, "", ""));
  remove.addEventListener("click", () => {
    row.remove();
    setWebhookHeaderStatus(form, "", "");
  });
  reveal?.addEventListener("click", () => {
    void revealWebhookHeader(form, row, reveal);
  });
}

function setupWebhookRetry(form: HTMLFormElement): void {
  const enabled = requiredElement<HTMLInputElement>(
    form,
    "[data-webhook-retry-enabled]",
  );
  const fields = requiredElement<HTMLElement>(
    form,
    "[data-webhook-retry-fields]",
  );

  const refresh = (): void => {
    fields.setAttribute("aria-disabled", enabled.checked ? "false" : "true");
  };

  enabled.addEventListener("change", refresh);
  refresh();
}

function hasWebhookTestProblem(problem: ProblemPayload): boolean {
  if (problem.error?.trim()) return true;
  return Object.values(problem.problems ?? {}).some(
    (message) => message.trim() !== "",
  );
}

// Sends one webhook test request and returns a structured server problem when present.
export async function sendWebhookTestRequest(
  action: string,
  body: URLSearchParams,
): Promise<ProblemPayload | null> {
  const response = await fetch(action, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      "Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
    },
    body,
  });
  if (response.ok) return null;

  const payload: unknown = await response
    .clone()
    .json()
    .catch(() => undefined);
  const problem = parseProblemPayload(payload);
  if (hasWebhookTestProblem(problem)) return problem;

  throw await responseProblem(response, payload);
}

function webhookTestFormBody(form: HTMLFormElement): URLSearchParams {
  const body = new URLSearchParams();

  for (const [name, value] of new FormData(form)) {
    if (typeof value === "string") body.append(name, value);
  }

  return body;
}

async function runWebhookTest(form: HTMLFormElement): Promise<void> {
  const submit = requiredElement<HTMLButtonElement>(
    form,
    'button[type="submit"]',
  );
  submit.disabled = true;

  try {
    const problem = await sendWebhookTestRequest(
      form.action,
      webhookTestFormBody(form),
    );
    if (problem) {
      const shown = await showProblemDialog(problem, {
        title: "Webhook test failed",
      });
      if (!shown) {
        await showNotice(
          problem.error?.trim() || "The webhook test could not be sent.",
          { title: "Webhook test failed" },
        );
      }
      return;
    }

    await showNotice("The webhook test was delivered successfully.", {
      title: "Webhook test sent",
    });
  } catch (error: unknown) {
    console.error("webhook test failed", error);
    await showNotice(
      errorMessage(error) || "The webhook test could not be sent.",
      { title: "Webhook test failed" },
    );
  } finally {
    submit.disabled = false;
  }
}

function setupWebhookTestForm(form: HTMLFormElement): void {
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void runWebhookTest(form);
  });
}

function setupWebhookForm(form: HTMLFormElement): void {
  const list = requiredElement<HTMLElement>(form, "[data-webhook-header-list]");
  const template = requiredElement<HTMLTemplateElement>(
    form,
    "[data-webhook-header-template]",
  );
  const addHeader = requiredElement<HTMLButtonElement>(
    form,
    "[data-webhook-header-add]",
  );

  list
    .querySelectorAll<HTMLElement>("[data-webhook-header-row]")
    .forEach((row) => bindWebhookHeaderRow(form, row));

  let nextHeaderRow = 1;
  addHeader.addEventListener("click", () => {
    const row = template.content.firstElementChild?.cloneNode(true) as
      HTMLElement | null | undefined;
    if (!row) return;

    assignWebhookHeaderRowNames(row, `n${nextHeaderRow++}`);
    list.append(row);
    bindWebhookHeaderRow(form, row);
    requiredElement<HTMLInputElement>(
      row,
      "[data-webhook-header-name]",
    ).focus();
  });

  setupWebhookRetry(form);
  setupWebhookTemplateAutocomplete(
    requiredElement<HTMLTextAreaElement>(form, "[data-webhook-template]"),
  );
}

// Initializes webhook controls for create and edit forms on the administration page.
export function initAdminWebhooks(): void {
  setupWebhookHelp();
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-webhook-form]",
  )) {
    setupWebhookForm(form);
  }
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-webhook-test-form]",
  )) {
    setupWebhookTestForm(form);
  }
}
