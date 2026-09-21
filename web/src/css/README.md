# CSS architecture

`app.css` is an import manifest only. The build concatenates the imported files in order; selectors in `app.css` are rejected by `scripts/web/build-css.sh`.

## Ownership

- `base/` — design tokens, resets, and tiny cross-cutting utilities.
- `layout/` — application-wide geometry such as the shell, main content area, and footer.
- `components/` — reusable UI primitives. Components own their chrome and expose CSS custom properties for variants instead of relying on source-order overrides.
- `markdown/` — rendered Markdown/prose presentation only.
- `navigation/` — sidebar/navigation behavior and preferences.
- `editor/` — editor-only controls and workspace presentation.
- `pages/` — route or surface-specific styles. Administration sub-features live under `pages/admin/`.
- `responsive/` — coordinated mobile behavior that spans multiple owners (shell, touch density, and feature interaction). Self-contained responsive behavior stays with its owner.
- `print/` — print-only overrides.

## Rules

1. Put a selector in the narrowest file that owns its behavior. Do not add page-specific selectors to generic component files.
2. Shared components define structure/chrome; variants set documented custom properties or modifier classes. For example, dialogs set `--dialog-width` rather than overriding the base `width`.
3. Avoid duplicate selectors across files when the later rule merely completes the earlier one. Consolidate the effective rule in its owner instead.
4. Keep feature-local breakpoints next to the feature. Use `responsive/mobile.css` only when the behavior coordinates multiple surfaces or global touch/layout behavior.
5. Do not put selectors in `app.css`; they would be invisible to the concatenating build and blur ownership.
