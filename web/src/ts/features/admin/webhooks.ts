// Administrator webhook payload, header, and retry configuration behavior.

import { requiredElement } from "../../core/dom.ts";
import { errorMessage, responseProblem } from "../../core/http.ts";

const sensitiveWebhookHeaderNames = new Set([
  "authorization",
  "cookie",
  "proxy-authorization",
  "x-api-key",
  "x-auth-token",
]);

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
}

// Initializes webhook controls for create and edit forms on the administration page.
export function initAdminWebhooks(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-webhook-form]",
  )) {
    setupWebhookForm(form);
  }
}
