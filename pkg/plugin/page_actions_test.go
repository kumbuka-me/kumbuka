package plugin

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageActionsResolveAndOrderLocalPaths(t *testing.T) {
	plugins := []LoadedPlugin{{
		Enabled: true,
		Manifest: pluginpackage.Manifest{
			ID: "io.example.actions",
			Modules: []pluginpackage.Module{
				{Type: "page-action", ID: "second", Name: "Second", URL: "/pages/${slug}?id=${id}", Order: 20},
				{Type: "page-action", ID: "first", Name: "First", URL: "/history/${slug}", Order: 10},
			},
		},
	}}

	actions := pageActions(plugins, 42, "guide/a b")
	require.Len(t, actions, 2)
	assert.Equal(t, "first", actions[0].ModuleID)
	assert.Equal(t, "/history/guide/a%20b", actions[0].URL)
	assert.Equal(t, "/pages/guide/a%20b?id=42", actions[1].URL)
}
