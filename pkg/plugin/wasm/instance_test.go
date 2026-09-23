package wasm

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContributionsDispatchesManifestModulesByType(t *testing.T) {
	t.Parallel()

	instance := &Instance{manifest: pluginpackage.Manifest{
		ID: "io.example.dispatch",
		Modules: []pluginpackage.Module{
			{Type: "admin-resource", ID: "records", Name: "Records"},
			{Type: "editor-insert", ID: "callout", Name: "Callout", Markdown: "> note"},
			{Type: "settings", ID: "feature", Name: "Feature"},
			{Type: "settings", ID: "configuration", Name: "Configuration", Fields: []pluginpackage.ConfigurationField{{ID: "value", Type: "text"}}},
			{Type: "content-style", ID: "style", CSS: ".example{}"},
			{Type: "render-policy", ID: "policy", Policy: "example"},
			{Type: "browser-module", ID: "browser", JavaScript: "export {}"},
			{Type: "renderer", ID: "pre", Stage: "preprocess"},
		},
	}}

	contributions := instance.Contributions()

	require.Len(t, contributions.AdminResources, 1)
	assert.Equal(t, "records", contributions.AdminResources[0].ID)
	require.Len(t, contributions.EditorInserts, 1)
	assert.Equal(t, "callout", contributions.EditorInserts[0].ID)
	require.Len(t, contributions.SettingsModules, 1)
	assert.Equal(t, "feature", contributions.SettingsModules[0].ID)
	require.Len(t, contributions.ContentStyles, 1)
	assert.Equal(t, "style", contributions.ContentStyles[0].ID)
	require.Len(t, contributions.RenderPolicies, 1)
	assert.Equal(t, "policy", contributions.RenderPolicies[0].ID)
	require.Len(t, contributions.BrowserModules, 1)
	assert.Equal(t, "browser", contributions.BrowserModules[0].ID)
	require.Len(t, contributions.Preprocessors, 1)
}
