package markdown

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCanPersistRejectsDynamicTemplateSyntaxOutsideFences verifies dynamic template syntax prevents persisted artifacts.
func TestCanPersistRejectsDynamicTemplateSyntaxOutsideFences(t *testing.T) {
	t.Parallel()
	renderer := NewWithRegistry(&plugin.Registry{})

	assert.False(t, renderer.CanPersist("Before {{var:name}} after", nil))
	assert.False(t, renderer.CanPersist("{{subpages}}", nil))
	assert.True(t, renderer.CanPersist("```go\nfmt.Println(\"{{literal}}\")\n```", nil))
	assert.True(t, renderer.CanPersist("# Static\n\nPlain Markdown.", nil))
}

// TestRenderFingerprintTracksBuildAndOptions verifies artifact fingerprints change with build identity and rendering options.
func TestRenderFingerprintTracksBuildAndOptions(t *testing.T) {
	t.Parallel()
	renderer := NewWithRegistry(&plugin.Registry{})
	renderer.SetArtifactBuild("v1.2.3", "abc123")

	base := DefaultOptions()
	first := renderer.RenderFingerprint(base)
	require.NotEmpty(t, first)

	changed := base
	changed.WikiLinks = !changed.WikiLinks
	assert.NotEqual(t, first, renderer.RenderFingerprint(changed))

	renderer.SetArtifactBuild("v1.2.4", "def456")
	assert.NotEqual(t, first, renderer.RenderFingerprint(base))
}

// TestDynamicReadPermissionIncludesNetworkHTTP verifies network-backed renders are never treated as persistable artifacts.
func TestDynamicReadPermissionIncludesNetworkHTTP(t *testing.T) {
	t.Parallel()
	assert.True(t, hasDynamicReadPermission([]string{"network:http"}))
	assert.True(t, hasDynamicReadPermission([]string{"users:read"}))
}
