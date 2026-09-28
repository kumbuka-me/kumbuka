// Start sandboxed plugin presentation as soon as the document has parsed.
// Keep this entrypoint deliberately small so read pages do not wait for the
// editor and administration module graph before interactive content appears.

import { renderPluginModules } from "./plugins/loader.ts";
import { hydrateDeferredFragments } from "./plugins/deferred.ts";

hydrateDeferredFragments(document);
await renderPluginModules(document);
