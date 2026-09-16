package handler

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginWidgetPreferencesExposeEnabledWidgets(t *testing.T) {
	t.Parallel()

	items := []plugin.LoadedPlugin{{
		Enabled: true,
		Manifest: pluginpackage.Manifest{
			ID:          "me.kumbuka.example",
			Name:        "Example",
			Description: "Example widgets.",
			Modules: []pluginpackage.Module{
				{Type: "widget", ID: "home", Surface: "home"},
				{Type: "widget", ID: "sidebar", Name: "Shortcut", Surface: "sidebar", Description: "Sidebar shortcut."},
			},
		},
	}, {
		Enabled: false,
		Manifest: pluginpackage.Manifest{
			ID:      "me.kumbuka.disabled",
			Modules: []pluginpackage.Module{{Type: "widget", ID: "home", Surface: "home"}},
		},
	}}

	preferences := pluginWidgetPreferences(items, []string{"me.kumbuka.example/sidebar"})

	require.Len(t, preferences, 2)
	assert.Equal(t, "Example", preferences[0].Label)
	assert.Equal(t, "Home", preferences[0].Surface)
	assert.True(t, preferences[0].Visible)
	assert.Equal(t, "Example · Shortcut", preferences[1].Label)
	assert.Equal(t, "Sidebar", preferences[1].Surface)
	assert.Equal(t, "Sidebar shortcut.", preferences[1].Description)
	assert.False(t, preferences[1].Visible)
}

func TestHiddenPluginWidgetsUpdatesPresentedWidgetsOnly(t *testing.T) {
	t.Parallel()

	items := []plugin.LoadedPlugin{{
		Enabled: true,
		Manifest: pluginpackage.Manifest{
			ID: "me.kumbuka.example",
			Modules: []pluginpackage.Module{
				{Type: "widget", ID: "home", Surface: "home"},
				{Type: "widget", ID: "sidebar", Surface: "sidebar"},
			},
		},
	}, {
		Enabled: false,
		Manifest: pluginpackage.Manifest{
			ID:      "me.kumbuka.disabled",
			Modules: []pluginpackage.Module{{Type: "widget", ID: "home", Surface: "home"}},
		},
	}}

	hidden := hiddenPluginWidgets(
		items,
		[]string{"me.kumbuka.example/home", "me.kumbuka.disabled/home", "me.kumbuka.removed/home"},
		[]string{"me.kumbuka.example/home", "me.kumbuka.example/sidebar"},
		[]string{"me.kumbuka.example/home"},
	)

	assert.Equal(t, []string{"me.kumbuka.example/sidebar", "me.kumbuka.disabled/home"}, hidden)
}
