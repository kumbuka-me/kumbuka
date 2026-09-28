// Browser entry point that initializes Kumbuka UI features.

import { initCommandPalette } from "./core/command-palette.ts";
import { initAboutDialog, initConfirmForms } from "./core/dialogs.ts";
import { initKeyboardWorkflow } from "./core/keyboard.ts";
import { initPerformance, measure, measureAsync } from "./core/performance.ts";
import { initPWA } from "./core/pwa.ts";
import { initTheme } from "./core/theme.ts";
import { initAdmin } from "./features/admin/index.ts";
import { initEditor } from "./features/editor/index.ts";
import { initExports } from "./features/exports.ts";
import { initGraph } from "./features/graph.ts";
import { initLayout } from "./features/layout.ts";
import { initMarkdown } from "./features/markdown.ts";
import { initMedia } from "./features/media.ts";
import { initMentionAutocomplete } from "./features/mentions.ts";
import { initNotifications } from "./features/notifications.ts";
import { initPageEditPresence } from "./features/page-edit-presence.ts";
import { initPage } from "./features/page.ts";
import { initPathPickers } from "./features/path-picker.ts";
import { initPluginInspectors } from "./features/plugin-inspectors.ts";
import { initSearch } from "./features/search.ts";
import { initTokens } from "./features/tokens.ts";

initPerformance();

await measureAsync("app-init", async () => {
  measure("path-pickers", initPathPickers);
  measure("theme", initTheme);
  measure("pwa", initPWA);
  measure("layout", initLayout);
  measure("about-dialog", initAboutDialog);
  measure("keyboard", initKeyboardWorkflow);
  measure("search", initSearch);
  measure("editor", initEditor);
  measure("media", initMedia);
  measure("tokens", initTokens);
  measure("exports", initExports);
  measure("plugin-inspectors", initPluginInspectors);
  measure("page", initPage);
  measure("page-edit-presence", initPageEditPresence);
  measure("graph", initGraph);
  measure("notifications", initNotifications);
  measure("mentions", initMentionAutocomplete);
  measure("confirm-forms", initConfirmForms);
  measure("command-palette", initCommandPalette);
  await measureAsync("markdown", initMarkdown);
  measure("admin", initAdmin);
});
