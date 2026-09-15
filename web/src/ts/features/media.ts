// Image upload, library, and Markdown insertion.

import { createLatestRequest } from "../core/async.ts";
import { requestConfirmation, showNotice } from "../core/dialogs.ts";
import { requiredAttribute, requiredElement } from "../core/dom.ts";
import { isRecord, requireArrayOf } from "../core/guards.ts";
import { errorMessage, requestJSON } from "../core/http.ts";
import { insertMarkdownAtSelection } from "./editor/toolbar.ts";

export type ImageItem = {
  id: number;
  filename: string;
  content_type: string;
  size_bytes: number;
  uploaded_by: number;
  uploader: string;
  created_at: string;
  usage_count: number;
  url: string;
};

function isImageItem(value: unknown): value is ImageItem {
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

function formatMediaSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;

  const units = ["KiB", "MiB", "GiB", "TiB"];
  let value = bytes / 1024;
  let unit = 0;

  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit += 1;
  }

  return `${value.toFixed(1)} ${units[unit]}`;
}

function formatMediaAge(value: string): string {
  const timestamp = new Date(value);
  const milliseconds = Date.now() - timestamp.getTime();
  if (!Number.isFinite(milliseconds)) return "";

  const minutes = Math.floor(milliseconds / 60_000);
  if (minutes < 1) return "just now";
  if (minutes < 60) return `${minutes}m ago`;

  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ago`;

  return timestamp.toISOString().slice(0, 10);
}

// Builds Markdown for an uploaded image.
export function imageMarkdown(
  image: Pick<ImageItem, "filename" | "url">,
): string {
  const label =
    image.filename
      .replace(/\.[^.]+$/, "")
      .replaceAll("[", "")
      .replaceAll("]", "") || "image";
  return `![${label}](${image.url})`;
}

// Uploads one image and returns its stored metadata.
async function uploadImage(url: string, file: File): Promise<ImageItem> {
  const data = new FormData();

  data.append("file", file);

  const payload = await requestJSON(url, {
    method: "POST",
    body: data,
  });
  if (!isImageItem(payload)) throw new Error("Invalid image response.");

  return payload;
}

// Returns image files from a file list.
function imageFiles(files: FileList | readonly File[]): File[] {
  return [...files].filter((file) => file.type.startsWith("image/"));
}

function hasDraggedImageItem(dataTransfer: DataTransfer | null): boolean {
  if (!dataTransfer) return false;

  return [...dataTransfer.items].some((item) => item.type.startsWith("image/"));
}

function hasDraggedImage(dataTransfer: DataTransfer | null): boolean {
  if (!dataTransfer) return false;
  if (imageFiles(dataTransfer.files).length > 0) return true;

  return hasDraggedImageItem(dataTransfer);
}

// Wires media dialog behavior.
function setupMediaDialog(dialog: HTMLDialogElement): void {
  const open = requiredElement<HTMLElement>(
    document,
    "[data-media-dialog-open]",
  );
  const close = requiredElement<HTMLButtonElement>(
    dialog,
    "[data-media-dialog-close]",
  );
  const body = requiredElement<HTMLElement>(dialog, "[data-media-dialog-body]");
  const uploadInput = requiredElement<HTMLInputElement>(
    dialog,
    "[data-media-upload-input]",
  );
  const uploadStatus = requiredElement<HTMLElement>(
    dialog,
    "[data-media-upload-status]",
  );
  const textarea = requiredElement<HTMLTextAreaElement>(
    document,
    "[data-markdown-editor]",
  );
  const sourcePane =
    textarea?.closest<HTMLElement>("[data-editor-source-pane]") ?? null;
  const dropStatus =
    sourcePane?.querySelector<HTMLElement>("[data-editor-upload-status]") ??
    null;
  const mediaBody = body;
  const mediaUploadInput = uploadInput;
  const mediaUploadStatus = uploadStatus;
  const editor = textarea;
  const mediaListURL = requiredAttribute(dialog, "data-media-list-url");
  const mediaUploadURL = requiredAttribute(dialog, "data-media-upload-url");

  let images: ImageItem[] = [];
  let loaded = false;
  let uploading = false;

  function renderImages(): void {
    mediaBody.replaceChildren();
    if (!images.length) {
      const empty = document.createElement("p");

      empty.className = "muted";
      empty.textContent = "No images uploaded yet.";
      mediaBody.append(empty);
      return;
    }

    for (const image of images) {
      const item = document.createElement("article");

      item.className = "media-dialog-item";

      const preview = document.createElement("img");

      preview.src = image.url;
      preview.alt = "";
      preview.loading = "lazy";

      const info = document.createElement("div");

      info.className = "media-dialog-info";

      const name = document.createElement("strong");

      name.textContent = image.filename;

      const meta = document.createElement("small");
      const references = `reference${image.usage_count === 1 ? "" : "s"}`;

      meta.textContent = `${formatMediaSize(image.size_bytes)} · ${image.usage_count} ${references}`;
      info.append(name, meta);

      const insert = document.createElement("button");

      insert.type = "button";
      insert.className = "button";
      insert.textContent = "Insert";
      insert.addEventListener("click", () => {
        insertMarkdownAtSelection(editor, imageMarkdown(image));
        dialog.close();
      });

      item.append(preview, info, insert);
      mediaBody.append(item);
    }
  }

  async function loadImages(force = false): Promise<void> {
    if (loaded && !force) return;

    mediaBody.innerHTML = '<p class="muted">Loading images…</p>';

    try {
      const payload = await requestJSON(mediaListURL);

      images = requireArrayOf(payload, isImageItem, "image list response");
      loaded = true;
      renderImages();
    } catch (error) {
      console.error("image library failed", error);
      mediaBody.innerHTML =
        '<p class="revision-dialog-error">Images could not be loaded.</p>';
    }
  }

  async function uploadFiles(
    files: FileList | readonly File[],
    { closeDialog = false }: { closeDialog?: boolean } = {},
  ): Promise<void> {
    const accepted = imageFiles(files);
    if (!accepted.length || uploading) return;

    uploading = true;
    mediaUploadInput.disabled = true;
    sourcePane?.classList.add("is-uploading-image");

    try {
      for (let index = 0; index < accepted.length; index += 1) {
        const file = accepted[index];
        const label = `Uploading image ${index + 1} of ${accepted.length}…`;

        mediaUploadStatus.textContent = label;

        if (dropStatus) dropStatus.textContent = label;

        const image = await uploadImage(mediaUploadURL, file);

        images.unshift(image);
        loaded = true;
        insertMarkdownAtSelection(editor, imageMarkdown(image));
      }

      renderImages();

      if (dropStatus) dropStatus.textContent = "Image uploaded and inserted.";
      if (closeDialog && dialog.open) dialog.close();
    } catch (error) {
      console.error("image upload failed", error);

      const message = errorMessage(error) || "Upload failed.";

      mediaUploadStatus.textContent = message;

      if (dropStatus) dropStatus.textContent = message;

      return;
    } finally {
      uploading = false;
      mediaUploadInput.disabled = false;
      mediaUploadInput.value = "";
      sourcePane?.classList.remove("is-uploading-image");
      sourcePane?.classList.remove("is-dragging-image");
    }

    mediaUploadStatus.textContent = "JPEG, PNG, GIF or WebP · max 10 MiB";

    if (dropStatus) {
      setTimeout(() => {
        dropStatus.textContent =
          "Paste or drop images directly into the editor.";
      }, 1800);
    }
  }

  open.addEventListener("click", () => {
    dialog.showModal();
    void loadImages();
  });
  close.addEventListener("click", () => dialog.close());
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) dialog.close();
  });

  mediaUploadInput.addEventListener("change", () => {
    if (mediaUploadInput.files)
      void uploadFiles(mediaUploadInput.files, { closeDialog: true });
  });

  editor.addEventListener("paste", (event: ClipboardEvent) => {
    const files = imageFiles(event.clipboardData?.files || []);
    if (!files.length) return;

    event.preventDefault();
    void uploadFiles(files);
  });

  if (sourcePane) {
    sourcePane.addEventListener("dragenter", (event: DragEvent) => {
      if (!hasDraggedImage(event.dataTransfer)) return;

      event.preventDefault();
      sourcePane.classList.add("is-dragging-image");
    });
    sourcePane.addEventListener("dragover", (event: DragEvent) => {
      if (!hasDraggedImageItem(event.dataTransfer)) return;

      event.preventDefault();

      if (event.dataTransfer) event.dataTransfer.dropEffect = "copy";

      sourcePane.classList.add("is-dragging-image");
    });
    sourcePane.addEventListener("dragleave", (event: DragEvent) => {
      const related = event.relatedTarget;
      if (!(related instanceof Node) || !sourcePane.contains(related))
        sourcePane.classList.remove("is-dragging-image");
    });
    sourcePane.addEventListener("drop", (event: DragEvent) => {
      const files = imageFiles(event.dataTransfer?.files || []);

      sourcePane.classList.remove("is-dragging-image");
      if (!files.length) return;

      event.preventDefault();
      editor.focus();
      void uploadFiles(files);
    });
  }
}

function managedImageEmptyText(mode: string, query: string): string {
  if (query) return "No images match your search.";
  if (mode === "admin") return "No images uploaded yet.";

  return "No images uploaded yet. Upload one from the page editor.";
}

function managedImageRow(image: ImageItem, mode: string): HTMLElement {
  const item = document.createElement("article");

  item.className = "media-settings-row";
  item.dataset.mediaSettingsItem = "";
  item.dataset.mediaId = String(image.id);

  const preview = document.createElement("img");

  preview.src = image.url;
  preview.alt = "";
  preview.loading = "lazy";

  const info = document.createElement("div");

  info.className = "media-settings-info";

  const name = document.createElement("strong");

  name.textContent = image.filename;

  const meta = document.createElement("small");
  const parts = [formatMediaSize(image.size_bytes)];

  if (mode === "admin") parts.push(image.uploader || "Unknown uploader");
  parts.push(formatMediaAge(image.created_at));
  meta.textContent = parts.filter(Boolean).join(" · ");

  const usage = document.createElement("small");

  if (image.usage_count > 0) {
    usage.className = "media-used";
    usage.textContent = `Referenced ${image.usage_count} time${image.usage_count === 1 ? "" : "s"}`;
  } else {
    usage.className = "media-unused";
    usage.textContent = "Unused";
  }

  const url = document.createElement("code");

  url.textContent = image.url;
  info.append(name, meta, usage, url);

  const actions = document.createElement("div");

  actions.className = "media-settings-actions";
  if (mode === "admin" || image.usage_count === 0) {
    const remove = document.createElement("button");

    remove.type = "button";
    remove.className = "button danger";
    remove.dataset.mediaDelete = "";
    remove.dataset.mediaUsage = String(image.usage_count);
    remove.dataset.deleteUrl = `/api/images/${image.id}`;
    remove.textContent = "Delete";
    remove.addEventListener("click", () => void deleteMediaImage(remove));
    actions.append(remove);
  }

  item.append(preview, info, actions);
  return item;
}

function updateManagedImageSummary(root: HTMLElement, hasMore: boolean): void {
  const count = root.querySelectorAll("[data-media-settings-item]").length;
  const summary = root.querySelector<HTMLElement>(
    "[data-media-settings-count]",
  );

  root.dataset.mediaOffset = String(count);
  root.dataset.mediaHasMore = String(hasMore);
  if (summary)
    summary.textContent = `${count} shown${hasMore ? " · more available" : ""}`;
}

function renderManagedImageEmpty(root: HTMLElement): void {
  const list = requiredElement<HTMLElement>(root, "[data-media-settings-list]");
  const input = root.querySelector<HTMLInputElement>(
    "[data-media-settings-search-input]",
  );
  const empty = document.createElement("p");

  empty.className = "muted";
  empty.dataset.mediaSettingsEmpty = "";
  empty.textContent = managedImageEmptyText(
    root.dataset.mediaMode || "user",
    input?.value.trim() || "",
  );
  list.append(empty);
}

function updateManagedImageURL(root: HTMLElement, query: string): void {
  const url = new URL(window.location.href);

  if (query) url.searchParams.set("image_q", query);
  else url.searchParams.delete("image_q");
  if (root.id) url.hash = root.id;

  window.history.replaceState(null, "", url);
}

function setupManagedImageBrowser(root: HTMLElement): void {
  const listURL = requiredAttribute(root, "data-media-list-url");
  const list = requiredElement<HTMLElement>(root, "[data-media-settings-list]");
  const input = requiredElement<HTMLInputElement>(
    root,
    "[data-media-settings-search-input]",
  );
  const form = requiredElement<HTMLFormElement>(
    root,
    "[data-media-settings-search]",
  );
  const clear = requiredElement<HTMLAnchorElement>(
    root,
    "[data-media-settings-clear]",
  );
  const loadMore = requiredElement<HTMLButtonElement>(
    root,
    "[data-media-load-more]",
  );
  const status = requiredElement<HTMLElement>(root, "[data-media-load-status]");
  const pageSize = Number(root.dataset.mediaPageSize) || 30;
  const mode = root.dataset.mediaMode || "user";
  const scope = root.dataset.mediaScope || "all";
  let loading = false;
  let activeQuery = input.value.trim();
  const requests = createLatestRequest();

  root.dataset.mediaHasMore = String(!loadMore.hidden);

  async function load(reset: boolean): Promise<void> {
    if (loading && !reset) return;

    const signal = requests.next();
    loading = true;
    loadMore.disabled = true;
    status.textContent = reset ? "Searching…" : "Loading…";

    const query = reset ? input.value.trim() : activeQuery;
    const offset = reset ? 0 : Number(root.dataset.mediaOffset || 0);
    const url = new URL(listURL, window.location.origin);

    url.searchParams.set("limit", String(pageSize + 1));
    url.searchParams.set("offset", String(offset));
    if (query) url.searchParams.set("q", query);
    if (scope === "mine") url.searchParams.set("scope", "mine");

    try {
      const payload = await requestJSON(url, { signal });
      if (signal.aborted) return;

      const page = requireArrayOf(payload, isImageItem, "image list response");
      const hasMore = page.length > pageSize;
      const visible = hasMore ? page.slice(0, pageSize) : page;

      if (reset) list.replaceChildren();
      list.querySelector("[data-media-settings-empty]")?.remove();
      activeQuery = query;
      for (const image of visible) list.append(managedImageRow(image, mode));
      if (!list.querySelector("[data-media-settings-item]"))
        renderManagedImageEmpty(root);

      updateManagedImageSummary(root, hasMore);
      loadMore.hidden = !hasMore;
      clear.hidden = query === "";
      status.textContent = "";
      updateManagedImageURL(root, query);
    } catch (error) {
      if (signal.aborted) return;

      console.error("managed image list failed", error);
      status.textContent = "";
      await showNotice(errorMessage(error) || "Images could not be loaded.", {
        title: "Image search failed",
      });
    } finally {
      if (!signal.aborted) {
        loading = false;
        loadMore.disabled = false;
      }
    }
  }

  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void load(true);
  });

  clear.addEventListener("click", (event) => {
    event.preventDefault();
    input.value = "";
    void load(true);
  });

  loadMore.addEventListener("click", () => void load(false));
}

async function deleteMediaImage(button: HTMLButtonElement): Promise<void> {
  const item = button.closest<HTMLElement>("[data-media-settings-item]");
  if (!item) return;

  const usage = Number(button.dataset.mediaUsage || 0);
  const message =
    usage > 0
      ? `Delete this image permanently? It is still referenced ${usage} time${usage === 1 ? "" : "s"} and those references will break.`
      : "Delete this unused image permanently?";
  if (
    !(await requestConfirmation(message, {
      title: "Delete image",
      confirmLabel: "Delete image",
    }))
  )
    return;

  const deleteURL = requiredAttribute(button, "data-delete-url");

  button.disabled = true;

  try {
    await requestJSON(deleteURL, { method: "DELETE" });

    const root = item.closest<HTMLElement>("[data-media-settings-browser]");

    item.remove();

    if (root) {
      const list = requiredElement<HTMLElement>(
        root,
        "[data-media-settings-list]",
      );
      const hasMore = root.dataset.mediaHasMore === "true";

      if (!list.querySelector("[data-media-settings-item]"))
        renderManagedImageEmpty(root);
      updateManagedImageSummary(root, hasMore);
    }
  } catch (error) {
    console.error("image deletion failed", error);
    await showNotice(errorMessage(error) || "Image could not be deleted.", {
      title: "Image deletion failed",
    });
    button.disabled = false;
  }
}

// Initializes media.
export function initMedia(): void {
  const mediaDialog = document.querySelector<HTMLDialogElement>(
    "[data-media-dialog]",
  );

  if (mediaDialog) setupMediaDialog(mediaDialog);

  for (const browser of document.querySelectorAll<HTMLElement>(
    "[data-media-settings-browser]",
  )) {
    setupManagedImageBrowser(browser);
  }

  for (const button of document.querySelectorAll<HTMLButtonElement>(
    "[data-media-delete]",
  )) {
    button.addEventListener("click", () => void deleteMediaImage(button));
  }
}
