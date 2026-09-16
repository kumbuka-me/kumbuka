package handler

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
)

func TestAddPluginFeaturesAddsGenericSyntaxSettings(t *testing.T) {
	features := map[string]bool{}
	addPluginFeatures(features, plugin.LoadedPlugin{
		Enabled: true,
		Manifest: pluginpackage.Manifest{
			ID: "io.example.tables",
			Modules: []pluginpackage.Module{
				{Type: "markdown-syntax", ID: "grammar", Syntax: "tables"},
				{Type: "settings", ID: "sorting", Requires: []string{"grammar"}},
				{Type: "settings", ID: "filtering", Requires: []string{"grammar"}},
			},
		},
		Settings: map[string]bool{"sorting": true, "filtering": false},
	})

	assert.True(t, features["io.example.tables"])
	assert.True(t, features["io.example.tables.sorting"])
	assert.False(t, features["io.example.tables.filtering"])
	assert.True(t, features["markdown-syntax.tables"])
	assert.True(t, features["markdown-syntax.tables.sorting"])
	assert.False(t, features["markdown-syntax.tables.filtering"])
}

func TestAddPluginFeaturesDoesNotExposeDisabledSyntax(t *testing.T) {
	features := map[string]bool{}
	addPluginFeatures(features, plugin.LoadedPlugin{
		Enabled: false,
		Manifest: pluginpackage.Manifest{
			ID:      "io.example.tables",
			Modules: []pluginpackage.Module{{Type: "markdown-syntax", ID: "grammar", Syntax: "tables"}},
		},
	})

	assert.False(t, features["io.example.tables"])
	assert.NotContains(t, features, "markdown-syntax.tables")
}
