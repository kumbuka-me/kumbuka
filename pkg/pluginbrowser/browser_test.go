package pluginbrowser

import (
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFrameEscapesAssetNamesAndKeepsExecutionIsolated(t *testing.T) {
	frame, policy, err := Frame("/plugins", "/plugins/runtime.js", []string{"https://kumbuka.test"}, plugin.BrowserContribution{PluginID: "io.example.test", ModuleID: "diagram", Digest: "abc", Name: "<script>alert(1)</script>", JavaScript: "entry#special?.js", CSS: "style%.css"})
	require.NoError(t, err)
	html := string(frame)
	assert.Contains(t, html, "entry%23special%3F.js")
	assert.Contains(t, html, "style%25.css")
	assert.NotContains(t, html, "<script>alert")
	assert.Contains(t, policy, "sandbox allow-scripts")
	assert.NotContains(t, policy, "allow-same-origin")
	assert.Contains(t, policy, "connect-src 'none'")
	assert.NotContains(t, html, `type="module"`)
	assert.NotContains(t, html, "crossorigin")
	assert.Contains(t, html, "[data-kumbuka-mention]")
	assert.Contains(t, html, "var(--accent)")
}
