// Editor attachment upload and insertion.

import { requestConfirmation } from "../../core/dialogs.ts";
import { requiredAttribute, requiredElement } from "../../core/dom.ts";
import { isRecord, requireArrayOf } from "../../core/guards.ts";
import { requestJSON } from "../../core/http.ts";
import { insertMarkdownAtSelection } from "./toolbar.ts";

export interface AttachmentItem {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  uploaded_by: number;
  uploader: string;
  created_at: string;
  usage_count: number;
  url: string;
}

function isAttachmentItem(value: unknown): value is AttachmentItem {
  return (
    isRecord(value) &&
    typeof value.id === "number" &&
    typeof value.filename === "string" &&
    typeof value.content_type === "string" &&
    typeof value.size_bytes === "number" &&
    typeof value.uploaded_by === "number" &&
    typeof value.uploader === "string" &&
    typeof value.created_at === "string" &&
    typeof value.usage_count === "number" &&
    typeof value.url === "string"
  );
}

// Builds the Markdown link for an attachment.
export function attachmentMarkdown(
  item: Pick<AttachmentItem, "filename" | "url">,
): string {
  return `[${item.filename}](${item.url})`;
}

// Wires attachment dialog behavior.
function setupAttachmentDialog(form: HTMLFormElement): void {
  const dialog = requiredElement<HTMLDialogElement>(
    document,
    "[data-attachment-dialog]",
  );
  const open = requiredElement<HTMLButtonElement>(
    form,
    "[data-attachment-dialog-open]",
  );
  const textarea = requiredElement<HTMLTextAreaElement>(
    form,
    "[data-markdown-editor]",
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-attachment-dialog-close]",
  );
  const body = requiredElement<HTMLElement>(
    dialog,
    "[data-attachment-dialog-body]",
  );
  const upload = requiredElement<HTMLInputElement>(
    dialog,
    "[data-attachment-upload]",
  );
  const status = requiredElement<HTMLElement>(
    dialog,
    "[data-attachment-status]",
  );
  const attachmentDialog = dialog;
  const attachmentListURL = requiredAttribute(
    dialog,
    "data-attachment-list-url",
  );
  const attachmentUploadURL = requiredAttribute(
    dialog,
    "data-attachment-upload-url",
  );
  const attachmentBody = body;
  const attachmentStatus = status;
  const editor = textarea;

  // Builds one attachment-library row.
  function row(item: AttachmentItem): HTMLElement {
    const element = document.createElement("div");

    element.className = "attachment-row";

    const info = document.createElement("span");
    const name = document.createElement("strong");
    const meta = document.createElement("small");

    name.textContent = item.filename;
    meta.textContent = `${Math.max(1, Math.round(item.size_bytes / 1024))} KiB · ${item.usage_count || 0} reference${item.usage_count === 1 ? "" : "s"}`;
    info.append(name, meta);

    const actions = document.createElement("span");

    actions.className = "attachment-row-actions";

    const insert = document.createElement("button");

    insert.type = "button";
    insert.className = "button";
    insert.textContent = "Insert";
    insert.addEventListener("click", () => {
      insertMarkdownAtSelection(editor, attachmentMarkdown(item));
      attachmentDialog.close();
    });

    const remove = document.createElement("button");

    remove.type = "button";
    remove.className = "button danger";
    remove.textContent = "Delete";
    remove.addEventListener("click", async () => {
      if (
        !(await requestConfirmation(
          `Delete ${item.filename}? Existing Markdown references will stop working.`,
          { title: "Delete attachment", confirmLabel: "Delete", danger: true },
        ))
      )
        return;

      try {
        await requestJSON(`/api/attachments/${item.id}`, { method: "DELETE" });
        element.remove();
      } catch (error) {
        console.error("attachment deletion failed", error);
      }
    });
    actions.append(insert, remove);
    element.append(info, actions);
    return element;
  }

  // Loads attachments into the library dialog.
  async function load(): Promise<void> {
    attachmentBody.innerHTML = '<p class="muted">Loading files…</p>';

    let items: AttachmentItem[];

    try {
      items = requireArrayOf(
        await requestJSON(attachmentListURL),
        isAttachmentItem,
        "attachment list response",
      );
    } catch (error) {
      console.error("attachment library failed", error);
      attachmentBody.innerHTML =
        '<p class="muted">Files could not be loaded.</p>';
      return;
    }

    attachmentBody.replaceChildren();
    if (!items.length) {
      const empty = document.createElement("p");

      empty.className = "muted";
      empty.textContent = "No attachments yet.";
      attachmentBody.append(empty);
      return;
    }

    items.forEach((item) => attachmentBody.append(row(item)));
  }

  open.addEventListener("click", () => {
    attachmentDialog.showModal();
    void load();
  });
  close.addEventListener("click", () => attachmentDialog.close());
  attachmentDialog.addEventListener("click", (event: MouseEvent) => {
    if (event.target === attachmentDialog) attachmentDialog.close();
  });
  upload.addEventListener("change", async () => {
    const file = upload.files?.[0];
    if (!file) return;

    const data = new FormData();

    data.append("file", file);
    attachmentStatus.textContent = `Uploading ${file.name}…`;

    let payload: unknown;

    try {
      payload = await requestJSON(attachmentUploadURL, {
        method: "POST",
        body: data,
      });
    } catch (error) {
      console.error("attachment upload failed", error);
      attachmentStatus.textContent = "Upload failed.";
      return;
    }

    if (!isAttachmentItem(payload)) {
      attachmentStatus.textContent = "Upload returned an invalid response.";
      return;
    }

    insertMarkdownAtSelection(editor, attachmentMarkdown(payload));
    upload.value = "";
    attachmentStatus.textContent = "Uploaded and inserted.";
    attachmentDialog.close();
  });
}

// Initializes attachments.
export function initAttachments(): void {
  for (const form of document.querySelectorAll<HTMLFormElement>(
    "[data-editor-form]",
  ))
    setupAttachmentDialog(form);
}
