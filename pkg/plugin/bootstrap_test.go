package plugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapUsesBundledPackageWhenInstalledDigestMatches(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	archive := lifecycleTestArchive(t, id, "1.0.0")
	store := &memoryStore{records: map[string]Record{
		id: {ID: id, Source: SourceInstalled, Enabled: false, Package: archive},
	}}
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, [][]byte{archive}))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, SourceBundled, loaded[0].Source)
	assert.False(t, loaded[0].Enabled)
	assert.Equal(t, packageDigest(t, archive), loaded[0].Digest)

	records, err := store.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, SourceInstalled, records[0].Source)
	assert.Equal(t, archive, records[0].Package)
	assert.False(t, records[0].Enabled)
}

func TestBootstrapKeepsInstalledPackageWhenSameVersionHasDifferentDigest(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	installed := lifecycleTestArchiveWithREADME(t, id, "1.0.0", "# Installed package\n")
	bundled := lifecycleTestArchiveWithREADME(t, id, "1.0.0", "# Bundled package\n")
	require.NotEqual(t, packageDigest(t, installed), packageDigest(t, bundled))

	store := &memoryStore{records: map[string]Record{
		id: {ID: id, Source: SourceInstalled, Enabled: true, Package: installed},
	}}
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, [][]byte{bundled}))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, SourceInstalled, loaded[0].Source)
	assert.Equal(t, "1.0.0", fmt.Sprint(loaded[0].Manifest.Version))
	assert.Equal(t, packageDigest(t, installed), loaded[0].Digest)
}

func TestBootstrapKeepsInstalledPackageWhenBundledDigestDiffers(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	installed := lifecycleTestArchive(t, id, "1.0.0")
	bundled := lifecycleTestArchive(t, id, "2.0.0")
	store := &memoryStore{records: map[string]Record{
		id: {ID: id, Source: SourceInstalled, Enabled: true, Package: installed},
	}}
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, [][]byte{bundled}))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, SourceInstalled, loaded[0].Source)
	assert.True(t, loaded[0].Enabled)
	assert.Equal(t, "1.0.0", fmt.Sprint(loaded[0].Manifest.Version))
	assert.Equal(t, packageDigest(t, installed), loaded[0].Digest)
	assert.NotEqual(t, packageDigest(t, bundled), loaded[0].Digest)
}

// packageDigest returns the validated SHA-256 archive digest used by plugin startup.
func packageDigest(t *testing.T, archive []byte) [32]byte {
	t.Helper()

	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	return pkg.Digest()
}
