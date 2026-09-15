package pluginproject

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectIncludesTransitiveDependencies(t *testing.T) {
	packages := []Resolved{
		{Manifest: pluginpackage.Manifest{ID: "io.base"}},
		{Manifest: pluginpackage.Manifest{ID: "io.child", Requires: []string{"io.base"}}},
		{Manifest: pluginpackage.Manifest{ID: "io.unused"}},
	}

	selected, err := Select(packages, []string{"io.child"})
	require.NoError(t, err)
	require.Len(t, selected, 2)
	assert.Equal(t, "io.base", selected[0].Manifest.ID)
	assert.Equal(t, "io.child", selected[1].Manifest.ID)
}
