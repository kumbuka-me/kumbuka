package handler

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
)

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
