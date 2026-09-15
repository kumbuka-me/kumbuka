// Ephemeral, sandboxed print previews. No values are stored in browser storage.

import { isRecord } from "../core/guards.ts";

export function parseExportPreview(value: unknown): string {
  if (
    !isRecord(value) ||
    typeof value.document !== "string" ||
    !value.document.trim()
  ) {
    throw new Error("Invalid export preview response.");
  }
  return value.document;
}

// A preview is trusted only because it came from Kumbuka's sanitized export
// endpoint. The sandbox still forbids scripts, forms, downloads and top-level
// navigation while allowing the parent to invoke the frame's print dialog.
export async function loadPrintPreview(
  host: HTMLElement,
  documentHTML: string,
  signal: AbortSignal,
): Promise<HTMLIFrameElement> {
  const frame = document.createElement("iframe");
  frame.className = "export-preview-frame";
  frame.title = "Print and PDF content preview";
  frame.setAttribute("sandbox", "allow-same-origin allow-modals");
  frame.referrerPolicy = "no-referrer";

  await new Promise<void>((resolve, reject) => {
    const timer = window.setTimeout(
      () => finish(new Error("The print preview could not finish loading.")),
      20000,
    );
    const aborted = () =>
      finish(new DOMException("Preview cancelled.", "AbortError"));
    const loaded = () => {
      if (frame.contentDocument?.URL !== "about:srcdoc") return;
      finish();
    };

    function finish(error?: Error): void {
      clearTimeout(timer);
      frame.removeEventListener("load", loaded);
      signal.removeEventListener("abort", aborted);
      if (error) {
        frame.remove();
        reject(error);
        return;
      }
      resolve();
    }

    frame.addEventListener("load", loaded);
    signal.addEventListener("abort", aborted, { once: true });
    if (signal.aborted) {
      aborted();
      return;
    }
    frame.srcdoc = documentHTML;
    host.replaceChildren(frame);
  });

  // The iframe load event waits for images. Fail visibly instead of printing a
  // silently broken image; font readiness prevents an early fallback-font print.
  const previewDocument = frame.contentDocument;
  if (!previewDocument) throw new Error("The print preview is not available.");
  await previewDocument.fonts.ready;
  if (signal.aborted) {
    throw new DOMException("Preview cancelled.", "AbortError");
  }
  for (const image of previewDocument.images) {
    if (!image.complete || image.naturalWidth === 0) {
      throw new Error("An image in the print preview could not be loaded.");
    }
  }
  return frame;
}
