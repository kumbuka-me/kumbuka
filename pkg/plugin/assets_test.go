package plugin

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

// TestBrowserCommandTargetsExposesSamePluginWidgets verifies browser modules can target host-mediated widget commands without learning unrelated plugin routes.
func TestBrowserCommandTargetsExposesSamePluginWidgets(t *testing.T) {
	manifest := pluginpackage.Manifest{
		Modules: []pluginpackage.Module{
			{Type: "browser-module", ID: "interactive", JavaScript: "plugin.js"},
			{Type: "widget", ID: "details", Surface: "page.details"},
			{Type: "renderer-extension", ID: "render", Stage: "preprocess"},
			{Type: "widget", ID: "sidebar", Surface: "sidebar"},
		},
	}

	targets := browserCommandTargets(manifest)

	require.Equal(t, []BrowserCommandTarget{
		{ModuleID: "details", Surface: "page.details"},
		{ModuleID: "sidebar", Surface: "sidebar"},
	}, targets)
}
