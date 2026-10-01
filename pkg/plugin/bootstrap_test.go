package plugin

import (
	"context"
	"fmt"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBootstrapKeepsInstalledPackageWhenBuiltinDigestMatches(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	archive := lifecycleTestArchive(t, id, "1.0.0")
	store := testStore(t, archive, false)
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{archive})))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
	assert.False(t, loaded[0].Enabled)
	assert.Equal(t, packageDigest(t, archive), loaded[0].Digest)

	records, err := store.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	stored, err := store.PluginPackage(ctx, id, packageDigest(t, archive))
	require.NoError(t, err)
	assert.Equal(t, archive, stored)
	assert.False(t, records[0].Enabled)
}

func TestBootstrapKeepsInstalledPackageWhenSameVersionHasDifferentDigest(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	installed := lifecycleTestArchiveWithREADME(t, id, "1.0.0", "# Installed package\n")
	bundled := lifecycleTestArchiveWithREADME(t, id, "1.0.0", "# Bundled package\n")
	require.NotEqual(t, packageDigest(t, installed), packageDigest(t, bundled))

	store := testStore(t, installed, true)
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{bundled})))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, "1.0.0", fmt.Sprint(loaded[0].Manifest.Version))
	assert.Equal(t, packageDigest(t, installed), loaded[0].Digest)
}

func TestBootstrapKeepsInstalledPackageWhenBundledDigestDiffers(t *testing.T) {
	t.Parallel()

	const id = "io.example.bootstrap"
	ctx := context.Background()
	installed := lifecycleTestArchive(t, id, "1.0.0")
	bundled := lifecycleTestArchive(t, id, "2.0.0")
	store := testStore(t, installed, true)
	manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{bundled})))

	loaded := manager.Plugins()
	require.Len(t, loaded, 1)
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

func testDistribution(t testing.TB, archives [][]byte) Distribution {
	t.Helper()
	d, err := NewArchiveDistribution(archives)
	require.NoError(t, err)
	return d
}
func testStore(t testing.TB, archive []byte, enabled bool) *memoryStore {
	t.Helper()
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	store := &memoryStore{records: make(map[string]Record)}
	require.NoError(t, store.SavePlugin(context.Background(), Record{ID: pkg.Manifest().ID, Manifest: pkg.Manifest(), Digest: pkg.Digest(), README: pkg.README(), Enabled: enabled}, archive))
	return store
}
