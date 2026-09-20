# Plugin architecture

Kumbuka's rendering modules register contributions through an application-owned
`plugin.Registry`. `markdown.New(ctx)` creates a renderer with an owned manager
and WASM runtime; `NewWithPluginStore` also restores persisted lifecycle state.
Callers handle startup errors and close the renderer at the end of its scope.
`markdown.NewWithRegistry` supports an explicitly owned registry.
Server pages, preview, sharing, and exports use the same pipeline; external tooling can reuse the public runtime packages.

Kumbuka's optional rendering and content features are bundled `.kumbukaplugin` packages
under `plugins/`. Executable plugins are independent Go modules; declarative-only
plugins contain only their manifest, documentation, and assets. Bundled and
installed packages use the same package reader, manager, registry, runtime,
permissions, settings, documentation, and asset paths.

## Packages and distribution

A `.kumbukaplugin` is a ZIP containing:

```text
README.md
plugin.yaml
plugin.wasm      # executable modules only
assets/          # optional package assets
```

The versioned manifest declares identity, a numeric `major.minor.patch` version,
modules, dependencies, presentation defaults, and permissions. The manifest
supports `renderer-extension` modules at `preprocess` and `postprocess` stages
and `macro` modules with parse/render calls. Unknown fields, unsupported versions, stages,
module types, and permission names are rejected. No capability is silently granted.

`pluginpackage.Read` validates the whole archive before returning immutable
content. Declarative-only packages omit `plugin.wasm`; packages containing a
renderer extension, macro, or code highlighter must include it. It rejects
traversal, absolute paths, backslashes, duplicate paths, nonregular files,
invalid directories, and file/directory collisions. It never
extracts into the filesystem. Limits are 16 MiB compressed, 32 MiB expanded,
256 entries, 16 MiB WASM, 8 MiB per asset, and 64 KiB for the manifest. CRC and
actual decompressed-size checks apply when entries are read.

`plugins` embeds downloaded release packages. Bundled startup and administrator
installation use the same package reader, runtime, and registry. `SourceBundled`
and `SourceInstalled` are descriptive metadata only: they do not change validation,
runtime configuration, permissions, or rendering behavior.

### Source-aware render planning

Executable manifest modules may declare cheap `usage` selectors (`contains`,
`macro`, inline `substitution`, or fenced-code `fence`). Kumbuka derives a versioned
page usage index when a persisted page is written and uses it to skip source-aware
modules that cannot participate in that page. The index is rebuildable metadata, never a second source
of truth. Preview and other non-persisted Markdown derive the same plan in memory,
and a stale index is ignored when the Markdown or active module selectors change.
Modules without selectors remain always active. Macro modules are source-aware automatically from
their declared macro name.

Content preprocessors are re-analyzed after a transformation so an include can
introduce later substitutions safely. Opaque substitutions and macro expansion
keep downstream parser/postprocessor stages conservative where inserted content
may contain additional syntax.

## Registration and lifetime

`Register(Descriptor, Contributions)` publishes a complete contribution set
atomically. Duplicate IDs and macro names, invalid metadata, and unavailable
dependencies fail without publishing partial contributions. Dependencies activate
first and retire after dependents.

Each top-level render acquires and releases one immutable `RenderPlan`. The plan is
rebuilt only when active plugin lifecycle state changes and contains only flattened
render contributions: pre-sorted content preprocessors, preprocessors, Markdown
extensions, the active highlighter, a macro-name index, postprocessors, rendering
policies, and source-usage descriptors. A request derives a smaller page render
plan from persisted/transient usage metadata, so unrelated modules are not walked
or invoked. Nested blocks and variable-provenance passes keep the same leased
global plan; registry locks are not held during rendering. Native callbacks must
be immutable and concurrency safe. Request macro bindings are copied and cannot
reactivate an unregistered macro.

`Manager.Install`, `Enable`, `Disable`, `Upgrade`, and `Uninstall` work inside
one running process. The candidate package is validated and instantiated before
publication. Registry validation, durable state commit, and publication are
ordered so validation or persistence failures leave the active version intact.
Replacement preserves contribution order and checks the complete dependency
graph, including cycles introduced by upgrades. Required plugins cannot be
disabled or removed while an enabled dependent still needs them.

A render plan leases every contribution version it references. Removing or replacing
an entry affects new renders immediately; the old WASM instance closes after its
last render releases the plan lease. Registry locks never cover guest execution.
Shutdown detaches all owned contributions atomically and waits for retired
instances. A cancelled shutdown can be retried. `Snapshot()` remains an unleased
inspection API; renderers use `AcquireRenderPlan()` and release on every path,
and all render execution uses `AcquireRenderPlan()`.

The manager's small `Store` interface persists installation records. Server
composition supplies PostgreSQL through `NewWithPluginStore`. The
`plugin_installations` table stores installed ZIP bytes separately from embedded
bundled packages, alongside source and enabled state. This uses Kumbuka's
persistence abstraction and requires no fixed filesystem path. The default
in-memory store supports isolated renderers and external tooling.

Startup merges bundled packages with persisted state, orders enabled plugins by
dependency, prepares every instance, and publishes the complete registry once.
An installed database package remains the explicit startup choice whenever its
SHA-256 digest differs from the embedded package with the same ID; versions are not
compared, so deliberate downgrades or pins remain stable across application upgrades.
If both archives have the same SHA-256 digest, startup uses the embedded copy and
applies the persisted enabled state without rewriting the database. Declarative-only
instances are constructed from the manifest without compiling or instantiating WASM.
A failed startup closes all prepared instances and publishes nothing. Bundled state
records never contain package bytes. Upgrading a bundled ID creates an installed
override through the same loader/runtime. Uninstall removes installed bytes; if that
ID has an embedded copy, the embedded copy remains disabled so a restart cannot
silently reactivate it. Plugin settings/data are retained for reinstallation. Version
replacement requires the same ID and preserves enabled state; explicit replacements
may also restore an earlier version.

The application exposes its manager through `Renderer.PluginManager()`.
Administration routes and UI use that same manager. External tooling that needs an
isolated package set can construct one through `markdown.NewWithPluginPackages`.

## Runtime boundary

`pkg/plugin/wasm` hosts WASI reactors with wazero. Each instance has its own
linear memory and serialized invocation gate. No host filesystem, environment,
arguments, sockets, or streams are configured. Only WASI and the signature-checked
`kumbuka_v1.call` import are accepted. Imported memories and cross-plugin module imports are rejected. API export
signatures and the guest's API version are checked before registration.

Defaults are 64 MiB guest memory, a 2-second call deadline, a 60-second
compilation/load deadline, a separate 2-second initialization deadline, 4 MiB
request/response and assembled-output limits, and 256 output fragments. Limits are
configurable at runtime construction. The renderer also limits recursive depth to 64 and a complete render to 30 seconds, respecting an
earlier request cancellation. Compilation has archive-size and elapsed-time
bounds, but not a separate hard cap on the compiler's host-memory usage.

Traps, deadline exhaustion, bad offsets, incompatible responses, and guest errors
become render errors. The bad reactor is closed; a later request can instantiate
a clean reactor from the compiled module. Nested Markdown fragments are handled
after the guest invocation, so there is no reentrant call into a Go WASM runtime.

Bounded process-local caches share validated package values and immutable compiled
modules, never active registries, guest state, or permissions. Compiled-module
leases keep an evicted entry alive until its active instances close. Wazero also
uses its filesystem-backed compilation cache below the operating system's normal
user cache directory (`kumbuka/wasm`), with an in-memory fallback when that directory
is unavailable. This avoids recompiling unchanged guests across normal process
restarts as well as repeated WASM decoding across renderer scopes. A startup gate
prevents duplicate concurrent compilations, following
[wazero's compilation-cache guidance](https://pkg.go.dev/github.com/tetratelabs/wazero#CompilationCache).

The wire ABI is documented by the public SDK in `github.com/kumbuka-me/sdk/WIRE.md`. Goldmark extension objects
remain host-side adapters; WASM guests never receive Goldmark or Kumbuka pointers.

## Rendering and sanitization

The pipeline recognizes registered macros outside CommonMark code, runs Markdown
preprocessors and core features, constructs fresh Goldmark extensions, expands
macros, then runs HTML postprocessors and the central sanitizer. Tabs/Details
ordering and macro-heading table-of-contents behavior are part of the pipeline.
Browser contributions run in isolated frames; editor and settings contributions
remain metadata.

Plugin lifecycle and declarative settings own plugin feature state. Core Markdown
semantics use fixed host defaults; optional feature toggles and typed singleton settings
are exposed and persisted through the generic plugin administration UI.

Every output path, including WASM fragments and macros, goes through the core
sanitizer. Plugins cannot mark HTML trusted or change the policy. A restricted,
static SVG geometry allowlist preserves navigation icons while rejecting active
SVG, URL-valued paints, scripts, and event handlers.

## Building and validation

First-party plugin source and deterministic package builds live in
`github.com/kumbuka-me/plugins` and `github.com/kumbuka-me/sdk`. This repository
pins released versions in `plugins.lock`; `make plugins` downloads the release
archives and `make generate` additionally regenerates the icon catalog. Runtime
tests execute released packages and adversarial WASM fixtures, including
ambient-capability denial, traps, malformed output, resource limits, request
isolation, and sanitizer enforcement.

## Capability and storage boundary

The public `github.com/kumbuka-me/sdk` module defines the JSON values and Go guest transport. It never imports
`pkg/domain`, `internal/postgres`, or `internal/http/endpoint`. `pkg/plugincap` is the
trusted composition adapter: it converts already-authorized catalogs and
navigation into public values. Normal pages, previews and exports keep the
viewer's access filter. Anonymous share scopes expose only the shared
page. Static rendering exposes prepared navigation with static URLs and leaves
unavailable query macros literal.

Runtime policy and manifest declarations must both allow each sensitive host
call. The caller identity is derived from the executing WASM instance and is
never accepted in request JSON. Capabilities are bound to the current invocation,
not stored in the instance; concurrent viewers cannot share callbacks. Calls
outside an invocation, unknown operations, malformed buffers, and undeclared
permissions are denied. A host-adapter panic becomes a guest-visible error.

The application explicitly grants page reads, namespaced settings/data, and
bounded infrastructure capabilities to requesting packages. The lower-level
runtime grants nothing by default. Attachments have an optional authorized
range-reader adapter and are not bound automatically. Generic outbound HTTP is
host-mediated: the plugin supplies the request, while Kumbuka enforces URL/header
bounds, redirect policy, timeouts, response limits, DNS/IP validation, proxy and
CA policy, and explicit permissions for private destinations or insecure origin
TLS. No raw socket, user directory, SQL, filesystem, or process capability is
available. The same grant rules apply to both distribution sources.

Storage is keyed by plugin ID, namespace, and key. Guest calls can reach only the
`settings` and `data` namespaces. Typed singleton `settings` groups and declarative
`admin-resource` records use reserved host-managed keys inside the owning plugin's
settings namespace. Singleton fields are read through `plugin.settings.read` as
`<module>.<field>` keys, while repeatable resources use the typed
`plugin.resources.get/list` capability. Host-managed keys cannot be overwritten through
the raw settings API. Manifest `secret` fields are encrypted before persistence, masked
in administration, and decrypted only when returned to the owning plugin. IDs come from the runtime. Limits are 256-byte keys, 64 KiB per
value, 1,024 keys and 16 MiB total per plugin. A PostgreSQL transaction and
per-plugin advisory lock make quota checks atomic. This storage survives runtime
restarts. Plugin installation state is stored separately.

See `github.com/kumbuka-me/sdk/WIRE.md` for methods and wire contracts. Tests cover actual WASM
macro parity between bundled and installed packages, authorization failures,
request isolation, host panics, malformed requests, namespace forgery, quota
checks, and PostgreSQL reopen persistence. Plugin package rebuilds happen in the plugin/SDK repositories after wire or guest-source changes.

## Lifecycle validation

Tests exercise install/disable/re-enable/upgrade/uninstall through real WASM in
one process, a render spanning an upgrade, dependency and cycle failures,
persistence failure rollback, bootstrap atomicity, installed overrides of bundled
IDs, and PostgreSQL/runtime reopen recovery.

## Browser modules

Mermaid is a bundled `.kumbukaplugin` with a WASM postprocessor and packaged
JavaScript/CSS. Bundled and installed modules use the same manager metadata,
versioned asset handlers and browser harness. `browser:render` must be declared
and granted. The manifest names `javascript` and optional `css` paths relative
to `assets/`; the loader validates that these files exist.

Enabled browser modules are embedded directly into each rendered page. Assets and
frames use `/plugins/{id}/{package-digest}/` URLs and are not cached. Disable or
upgrade invalidates old asset URLs, but already-open pages keep the catalog they
were rendered with. Reload the page to pick up plugin lifecycle changes. A failed
browser module still restores its source fallback.

Core accepts sanitized blocks marked with `data-kumbuka-plugin` and
`data-kumbuka-module`, containing a direct `pre` child. It passes only that block's
text and theme to an opaque sandbox iframe. Plugin JavaScript defines
`globalThis.kumbukaPlugin.render(root, {source, theme})`, optionally returning a
promise. The classic-script contract works on static hosts without CORS setup.
The shared core harness reports readiness, failure and bounded height, with constrained user-click forwarding for links already in the original
fallback; plugin HTML is never inserted into Kumbuka's parent document.

Frames allow scripts but not same-origin privileges, parent DOM access, forms,
popups or top navigation. CSP limits script/style resources to the package and
core harness, denies fetch/connect, and permits only data images/fonts. This is
browser isolation, not a general network sandbox: browser-controlled frame
navigation is not fully preventable across engines. Browser modules should not
receive secrets; they receive only their rendered block. WASM remains subject to
the separate host capability and resource limits.

External static builders can copy package assets and frame documents from the
selected renderer while preserving the same sandbox policy. Export/PDF documents
remain script-free and preserve diagram source as their source fallback.

## Tables and public rendering declarations

Tables ships as a bundled package. Its manifest owns standard table syntax
activation, the WASM directive stages, browser module and settings relationships
(`styles`, `sorting`, `filtering` require `tables`). The renderer and HTTP handlers
do not implement table syntax, directives, interactions or dependency rules.

Standard grammar declarations are intentionally host parser primitives available
to every package. This avoids a second Markdown parser in WASM that would lose
wiki links, variables, references and other plugin syntax inside cells. Selection
comes from the manifest and registry, never a switch on a bundled plugin ID.
WASM code owns all custom directive and presentation work. Core retains the
sanitizer and supplies no privileged native callback to the Tables guest.

Plain tables remain native semantic HTML. Interactive tables pass a sanitized
HTML copy to the sandbox. Disable or a failed browser module restores the original
HTML; print and script-free PDF/export keep semantic tables. External static builders
can copy the same package and core-filtered color stylesheet. Unsupported external or SVG
images keep an interactive table in its native fallback, avoiding network grants
to browser plugins. Same-origin raster images have strict transfer limits.

`/plugins/styles.css` emits only core-filtered, plugin-scoped color declarations
from enabled packages. Arbitrary stylesheet rules stay in frames. This preserves
fallback colors without letting community CSS modify Kumbuka's surrounding UI.

## Icon resources

Enabled packages may contribute icons to Kumbuka's shared icon catalog with the
SDK-defined `icon-resource` module. The module names the picker source and explicitly
declares the packaged JSON asset:

```yaml
modules:
  - type: icon-resource
    id: example-icons
    name: Example Icons
    asset: icons.json
```

The asset is declarative and does not grant a capability. Core strictly decodes it,
rejects malformed entries, and emits the SVG wrapper itself. Plugin markup is never
trusted as `template.HTML`.

```json
{
  "format": 1,
  "icons": [
    {
      "name": "example-brand",
      "label": "Example",
      "view_box": "0 0 24 24",
      "paths": ["M12 0L24 24H0z"]
    }
  ]
}
```

Names share the normal persisted icon namespace, so built-in Lucide identifiers win
on collisions. The merged catalog is cached by active plugin generation and is used by
the picker, persisted-icon validation, templates, external builders, and the `icons.render`
host capability. Disabling or removing the owning plugin removes its icons immediately.

## Administration

`/admin/plugins` and the dedicated `/admin/plugin-settings/{pluginID}` pages
use browser authentication and administrator authorization middleware. The Plugins
page owns package lifecycle; the settings pages are generated from manifest
`settings`, `admin-resource`, and `admin-action` declarations. Bounded multipart uploads call the
same manager methods as runtime callers; handlers neither extract files nor
instantiate separate runtimes. Invalid packages, permission failures and lifecycle
errors leave state unchanged. Request handlers log successful lifecycle actions
with actor and plugin IDs; internal failure details remain in server logs.

The optional manifest `provider` is bounded, self-declared display metadata.
`WithRequiredPlugins` is trusted operator policy, independent of source and
permissions. Bootstrap enables required IDs (and rejects missing ones), and all
public disable and uninstall paths enforce the policy. Shutdown still closes
required instances normally. No bundled feature is required by default.

The admin UI renders each package `README.md` and exposes enable/disable lifecycle
controls from the Plugins page. Plugins that declare boolean feature-toggle `settings`,
typed singleton `settings`, structured `admin-resource`, or executable `admin-action` modules
appear in a separate Plugin settings section of the administration sidebar. Boolean feature
toggles reach renderers as generic feature flags. Typed settings and resources support bounded
text, textarea, URL, secret, boolean, and select fields without teaching core plugin-specific
configuration names. Administrator actions are explicit manifest-declared buttons whose WASM
handlers run only for active plugins inside the authenticated administrator request context.

## Core page primitives

Wiki links remain core, alongside CommonMark. Page mutations extract canonical
targets with `markdown.Links` before persistence (`internal/application/pages/mutations.go`),
and other consumers use the same shared Markdown package. Plugin activation must not
change the persisted page-link graph or the meaning of stored page references. The
Rendering preference controls their presentation; it does not disable extraction.
Optional Markdown features are declared by plugins and administered in their
plugin details. Public grammar and rendering-policy declarations are translated
by the host; they do not grant native code access. Normal pages, previews, PDF exports, and external tooling all consume the same renderer and immutable registry render plan.

## Go developer boundary

The public `github.com/kumbuka-me/sdk` module owns Go reactor exports, guest
buffers, typed capabilities, macro serialization, package validation, and the
`kumbuka-plugin` CLI. First-party plugin source lives in the separate
`github.com/kumbuka-me/plugins` repository. Kumbuka pins released plugin versions
in `plugins.lock`, downloads their verified `.kumbukaplugin` assets, and embeds
those distribution bytes through the ordinary bootstrap path.

See the SDK repository at `https://github.com/kumbuka-me/sdk` for project
scaffolding, testing, wire contracts, and deterministic packaging.
