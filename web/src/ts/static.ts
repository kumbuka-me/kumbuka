// Static-site entry point. Initializes read-only Kumbuka UI features only.

import { initTheme } from "./core/theme.ts";
import { initMarkdown } from "./features/markdown.ts";
import { initStaticLayout } from "./features/static-layout.ts";
import { initStaticPage } from "./features/static-page.ts";
import { initStaticSearch } from "./features/static-search.ts";

initTheme();
initStaticLayout();
initStaticPage();
initStaticSearch();
await initMarkdown();
