# Documentation screenshots

Kumbuka owns the browser and application-side screenshot driver, but it does not own documentation content.

The driver imports Markdown supplied by another repository, starts a temporary PostgreSQL-backed Kumbuka instance, and captures the dashboard and editor with Playwright.

From the Kumbuka repository:

```sh
make screenshots \
  SCREENSHOT_CONTENT=../docs/content \
  SCREENSHOT_OUTPUT=../docs/assets/screenshots \
  SCREENSHOT_EDITOR_SLUG=getting-started \
  SCREENSHOT_VISITS=/pages/getting-started,/pages/content/editor,/pages/knowledge/search
```

The documentation repository is expected to provide the canonical Markdown and choose which pages populate the screenshot scenario.
