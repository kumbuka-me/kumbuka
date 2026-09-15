package plugin

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFeatureDependenciesUseRegistryMetadata(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register(Descriptor{ID: "community", Name: "Example"}, Contributions{SettingsModules: []SettingsModule{{ID: "colors", Name: "Colors", Requires: []string{"grammar"}}}}))
	snapshot := registry.Snapshot()
	require.ErrorContains(t, snapshot.ValidateFeatures(map[string]bool{"community.grammar": false}), "Colors requires grammar")
	require.NoError(t, snapshot.ValidateFeatures(map[string]bool{"community.grammar": false, "community.colors": false}))
	snapshot.Entries[0].Contributions.SettingsModules[0].Requires[0] = "changed"
	assert.Equal(t, "grammar", registry.Snapshot().Entries[0].Contributions.SettingsModules[0].Requires[0])
}
