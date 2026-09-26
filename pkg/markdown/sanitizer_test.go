package markdown

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestSanitizerKeepsInlineBrowserModuleMarkers verifies inline fallbacks can opt into the isolated browser-module loader.
func TestSanitizerKeepsInlineBrowserModuleMarkers(t *testing.T) {
	input := `<span class="status" data-kumbuka-plugin="me.kumbuka.status-dropdowns" data-kumbuka-module="status-ui" data-kumbuka-input="html"><span data-kumbuka-fallback>Done</span></span>`

	output := newSanitizer().Sanitize(input)

	require.Contains(t, output, `data-kumbuka-plugin="me.kumbuka.status-dropdowns"`)
	require.Contains(t, output, `data-kumbuka-module="status-ui"`)
	require.Contains(t, output, `data-kumbuka-input="html"`)
	require.Contains(t, output, `data-kumbuka-fallback`)
}

// TestSanitizerRejectsInvalidInlineBrowserModuleMarkers verifies malformed module identities are removed from inline content.
func TestSanitizerRejectsInvalidInlineBrowserModuleMarkers(t *testing.T) {
	input := `<span data-kumbuka-plugin="../other" data-kumbuka-module="status ui" data-kumbuka-input="javascript"><span data-kumbuka-fallback>Done</span></span>`

	output := newSanitizer().Sanitize(input)

	require.NotContains(t, output, `data-kumbuka-plugin`)
	require.NotContains(t, output, `data-kumbuka-module`)
	require.NotContains(t, output, `data-kumbuka-input`)
	require.Contains(t, output, `data-kumbuka-fallback`)
}

// TestSanitizerKeepsCoreMentionMarker verifies plugins can opt into the host-owned mention presentation contract.
func TestSanitizerKeepsCoreMentionMarker(t *testing.T) {
	input := `<span data-kumbuka-mention>@alice</span>`

	output := newSanitizer().Sanitize(input)

	require.Contains(t, output, `data-kumbuka-mention`)
	require.Contains(t, output, `@alice`)
}

// TestSanitizerRestrictsCoreMentionMarkerToSpans keeps the presentation primitive bounded to inline mention content.
func TestSanitizerRestrictsCoreMentionMarkerToSpans(t *testing.T) {
	input := `<div data-kumbuka-mention>@alice</div>`

	output := newSanitizer().Sanitize(input)

	require.NotContains(t, output, `data-kumbuka-mention`)
	require.Contains(t, output, `@alice`)
}
