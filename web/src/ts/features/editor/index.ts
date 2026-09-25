// Editor feature bootstrap.

import { setupIconPicker } from "../../core/icon-picker.ts";
import { initEditorExperience } from "./experience.ts";
import { initEditorIntelligence } from "./intelligence.ts";
import { initAttachments } from "./attachments.ts";
import { initSlashCommands } from "./commands.ts";
import { initEditorPreview } from "./preview.ts";
import { initEditorSearch } from "./search.ts";
import { initTags } from "./tags.ts";
import { initTablePalette } from "./tables.ts";
import { initTablePaste } from "./paste-table.ts";
import { initMarkdownListContinuation } from "./lists.ts";
import { initMarkdownToolbar } from "./toolbar.ts";
import { initLazyVisualEditor } from "./visual-loader.ts";
import { initVisualWidgetPopovers } from "./visual-widget-popover.ts";
import { initWikiLinkAutocomplete } from "./wikilinks.ts";
import { initPluginCompletions } from "./completions.ts";

// Initializes editor.
export function initEditor(): void {
  const pageIconPicker = document.querySelector<HTMLDialogElement>(
    "[data-page-icon-picker-dialog]",
  );

  if (pageIconPicker) setupIconPicker(pageIconPicker);

  initTags();
  initAttachments();
  initMarkdownToolbar();
  initWikiLinkAutocomplete();
  initPluginCompletions();
  initSlashCommands();
  initMarkdownListContinuation();
  initTablePalette();
  initTablePaste();
  initVisualWidgetPopovers();
  // Visual mode must register before preview captures the mode switch buttons.
  initLazyVisualEditor();
  initEditorPreview();
  initEditorSearch();
  initEditorIntelligence();
  initEditorExperience();
}
