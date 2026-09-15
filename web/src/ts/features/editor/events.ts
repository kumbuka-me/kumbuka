// Shared editor event contracts for producers and consumers.

export type DraftValues = Record<string, string[]>;

export interface EditorRestoreDraftDetail {
  values: DraftValues;
}

type EditorSignalEvent = "editor:find" | "editor:toggle-preview";

declare global {
  interface HTMLElementEventMap {
    "editor:restore-draft": CustomEvent<EditorRestoreDraftDetail>;
    "editor:find": CustomEvent<void>;
    "editor:toggle-preview": CustomEvent<void>;
  }
}

export function dispatchEditorEvent(
  target: HTMLElement,
  name: EditorSignalEvent,
): boolean;
export function dispatchEditorEvent(
  target: HTMLElement,
  name: "editor:restore-draft",
  detail: EditorRestoreDraftDetail,
): boolean;
export function dispatchEditorEvent(
  target: HTMLElement,
  name: keyof HTMLElementEventMap,
  detail?: EditorRestoreDraftDetail,
): boolean {
  return target.dispatchEvent(
    detail === undefined
      ? new CustomEvent(name)
      : new CustomEvent(name, { detail }),
  );
}
