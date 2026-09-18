package pluginbrowser

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestPresentationStylesVersionIsStableWithoutContributions verifies empty presentation catalogs have a stable fingerprint.
func TestPresentationStylesVersionIsStableWithoutContributions(t *testing.T) {
	t.Parallel()

	version := PresentationStylesVersion(nil)

	assert.Len(t, version, 16)
	assert.Equal(t, version, PresentationStylesVersion(nil))
}

// TestPresentationStylesAreScopedColorsOnly verifies browser stylesheet publication remains limited to scoped colors.
func TestPresentationStylesAreScopedColorsOnly(t *testing.T) {
	result := scopedColors("io.example.test", `
 @import url(https://evil.test/);
 @media screen { body { color: red; } }
 body, .prose .tone { background:color-mix(in srgb,var(--accent) 25%,var(--surface)); position:fixed; inset:0; display:none; }
 .bad { background:url(https://evil.test/); color:expression(evil); border-color: red; }
 .escape:has(*) { color:red; }
 .generated::before { content:'secret'; color:blue; }
 `)
	assert.Contains(t, result, `[data-kumbuka-plugin="io.example.test"] .tone`)
	assert.Contains(t, result, "background-color:color-mix")
	assert.Contains(t, result, "border-color:red")
	for _, forbidden := range []string{"@import", "@media", "position", "inset", "display", "url(", "expression", "content:", ":has"} {
		assert.NotContains(t, result, forbidden)
	}
}

// TestContentStylesAllowSafeRenderedPresentation verifies content plugins can use the bounded local-layout presentation subset.
func TestContentStylesAllowSafeRenderedPresentation(t *testing.T) {
	result := scopedContentStyles(`
.prose, .prose code, .admin-page {
  font-family: "Fira Code", monospace;
  font-variant-ligatures: contextual;
  background: url(https://evil.test/);
}
.prose pre { font-feature-settings: "calt" 1, "liga" 1; }
.prose .task-list-item { list-style: none; position: fixed; }
.prose .task-list-checkbox { margin-right: 0.45em; }
.prose .task-list-checkbox.checked { color: var(--accent); }
.prose .plugin-card { display: flex; gap: 0.5rem; padding-left: 0.75rem; border-width: 1px; border-style: solid; border-color: var(--border); border-radius: 0.5rem; background-color: var(--surface); overflow: hidden; }
.prose .plugin-card .body { flex: 1 1 auto; min-width: 0; white-space: pre; overflow-wrap: anywhere; font-weight: 600; }
.prose .bad:hover { color: red; }
`)

	assert.Contains(t, result, `.prose`)
	assert.Contains(t, result, `.prose code`)
	assert.Contains(t, result, `.prose pre`)
	assert.Contains(t, result, `.prose .task-list-item{list-style:none;}`)
	assert.Contains(t, result, `.prose .task-list-checkbox{margin-right:0.45em;}`)
	assert.Contains(t, result, `.prose .task-list-checkbox.checked{color:var(--accent);}`)
	assert.Contains(t, result, `.prose .plugin-card{display:flex;gap:0.5rem;padding-left:0.75rem;border-width:1px;border-style:solid;border-color:var(--border);border-radius:0.5rem;background-color:var(--surface);overflow:hidden;}`)
	assert.Contains(t, result, `.prose .plugin-card .body{flex:1 1 auto;min-width:0;white-space:pre;overflow-wrap:anywhere;font-weight:600;}`)
	assert.Contains(t, result, `font-family:`)
	assert.Contains(t, result, `font-variant-ligatures:contextual;`)
	assert.Contains(t, result, `font-feature-settings:`)
	for _, forbidden := range []string{".admin-page", ":hover", "background:url", "position", "url("} {
		assert.NotContains(t, result, forbidden)
	}
}

// TestCodeStylesAreScopedAndPresentationOnly verifies code-highlighter styles remain scoped and presentation-only.
func TestCodeStylesAreScopedAndPresentationOnly(t *testing.T) {
	result := scopedCodeStyles("io.example.highlight", `
.prose .chroma { background: var(--surface-hover); color: var(--text); position: fixed; }
.prose .chroma .k { color: var(--accent); font-weight: 600; font-style: italic; }
.prose .bad { background: url(https://evil.test/); display: none; }
`)

	assert.Contains(t, result, `[data-kumbuka-plugin="io.example.highlight"] .chroma`)
	assert.Contains(t, result, "background-color:var(--surface-hover);")
	assert.Contains(t, result, "font-weight:600;")
	assert.Contains(t, result, "font-style:italic;")
	for _, forbidden := range []string{"position", "url(", "display"} {
		assert.NotContains(t, result, forbidden)
	}
}
