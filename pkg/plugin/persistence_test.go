package plugin

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type observedStore struct {
	*memoryStore
	reads  []string
	saves  int
	states int
	fail   bool
}

func (s *observedStore) PluginPackage(ctx context.Context, id string, digest [32]byte) ([]byte, error) {
	s.reads = append(s.reads, id)
	return s.memoryStore.PluginPackage(ctx, id, digest)
}

func (s *observedStore) SavePlugin(ctx context.Context, r Record, b []byte) error {
	if s.fail {
		return errors.New("persistence failed")
	}
	s.saves++
	return s.memoryStore.SavePlugin(ctx, r, b)
}

func (s *observedStore) SeedPlugin(ctx context.Context, r Record, b []byte) error {
	if s.fail {
		return errors.New("persistence failed")
	}
	s.saves++
	return s.memoryStore.SeedPlugin(ctx, r, b)
}

func (s *observedStore) SetPluginEnabled(ctx context.Context, id string, b bool) error {
	if s.fail {
		return errors.New("persistence failed")
	}
	s.states++
	return s.memoryStore.SetPluginEnabled(ctx, id, b)
}

type observedRuntime struct {
	loads     []string
	instances []*observedInstance
	fail      string
}

func (r *observedRuntime) Load(_ context.Context, p *pluginpackage.Package) (Instance, error) {
	r.loads = append(r.loads, p.Manifest().ID)
	if p.Manifest().ID == r.fail {
		return nil, errors.New("initialization failed")
	}
	i := &observedInstance{}
	r.instances = append(r.instances, i)
	return i, nil
}
func (*observedRuntime) Close(context.Context) error { return nil }

type observedInstance struct{ closed bool }

func (*observedInstance) Contributions() Contributions  { return Contributions{} }
func (i *observedInstance) Close(context.Context) error { i.closed = true; return nil }
func inventoryArchive(t *testing.T, id, version string, enabled bool, requires string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range map[string]string{"README.md": "fixture", "plugin.yaml": fmt.Sprintf("api_version: 1\nid: %s\nname: Fixture\nversion: %s\ndefault_enabled: %t\nrequires: [%s]\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\n", id, version, enabled, requires)} {
		f, err := w.Create(name)
		require.NoError(t, err)
		_, err = f.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return b.Bytes()
}

func TestMetadataInventoryAndEagerActivation(t *testing.T) {
	ctx := context.Background()
	disabled := inventoryArchive(t, "io.disabled", "1.0.0", false, "")
	enabled := inventoryArchive(t, "io.enabled", "1.0.0", true, "")
	store := &observedStore{memoryStore: &memoryStore{records: make(map[string]Record)}}
	runtime := &observedRuntime{}
	manager := NewManager(&Registry{}, runtime, WithStore(store))
	distribution := testDistribution(t, [][]byte{disabled, enabled})
	require.NoError(t, manager.Bootstrap(ctx, distribution))
	assert.Equal(t, 2, store.saves)
	assert.Equal(t, []string{"io.enabled"}, store.reads)
	assert.Equal(t, []string{"io.enabled"}, runtime.loads)
	require.NoError(t, manager.Close(ctx))
	store.reads = nil
	runtime = &observedRuntime{}
	manager = NewManager(&Registry{}, runtime, WithStore(store))
	require.NoError(t, manager.Bootstrap(ctx, distribution))
	assert.Equal(t, 2, store.saves, "restart must not replace installations")
	assert.Equal(t, []string{"io.enabled"}, store.reads)
	require.NoError(t, manager.Enable(ctx, "io.disabled"))
	assert.Equal(t, []string{"io.enabled", "io.disabled"}, store.reads)
	assert.Equal(t, []string{"io.enabled", "io.disabled"}, runtime.loads)
	require.NoError(t, manager.Disable(ctx, "io.disabled"))
	assert.Equal(t, 2, store.saves, "enable and disable only update lifecycle state")
	assert.Equal(t, 2, store.states)
	assert.True(t, runtime.instances[1].closed)
	require.NoError(t, manager.Close(ctx))
}

func TestBuiltinReconciliationPreservesInstalledIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, installed, builtin string
		mismatch, update         bool
	}{
		{"newer builtin", "1.9.0", "1.10.0", false, true},
		{"pinned newer installed", "1.11.0", "1.10.0", false, false},
		{"same version mismatch", "1.10.0", "1.10.0", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			id := "io.fixture"
			installed := lifecycleTestArchiveWithREADME(t, id, tc.installed, "installed")
			builtin := lifecycleTestArchiveWithREADME(t, id, tc.builtin, "builtin")
			store := &observedStore{memoryStore: testStore(t, installed, false)}
			manager := NewManager(&Registry{}, lifecycleTestRuntime{}, WithStore(store))
			require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{builtin})))
			metadata := manager.Plugins()[0]
			assert.Equal(t, tc.installed, metadata.Manifest.Version)
			assert.Equal(t, tc.update, metadata.BuiltinUpdateAvailable)
			assert.Equal(t, tc.mismatch, metadata.BuiltinMismatch)
			assert.Empty(t, store.reads)
			assert.Zero(t, store.saves)
			if tc.update {
				candidate, err := manager.BuiltinArchive(ctx, id, tc.builtin)
				require.NoError(t, err)
				_, err = manager.Upgrade(ctx, id, candidate)
				require.NoError(t, err)
				assert.Equal(t, tc.builtin, manager.Plugins()[0].Manifest.Version)
				assert.False(t, manager.Plugins()[0].BuiltinUpdateAvailable)
				persisted, err := store.PluginPackage(ctx, id, packageDigest(t, builtin))
				require.NoError(t, err)
				assert.Equal(t, builtin, persisted)
			}
			require.NoError(t, manager.Close(ctx))
		})
	}
}

func TestLaterDistributionSeedsOnlyNewIDs(t *testing.T) {
	ctx := context.Background()
	existing := inventoryArchive(t, "io.existing", "1.0.0", false, "")
	added := inventoryArchive(t, "io.added", "1.0.0", true, "")
	thirdParty := inventoryArchive(t, "io.thirdparty", "1.0.0", true, "")
	store := &observedStore{memoryStore: testStore(t, existing, false)}
	pkg, err := pluginpackage.Read(thirdParty)
	require.NoError(t, err)
	require.NoError(t, store.memoryStore.SavePlugin(ctx, Record{ID: pkg.Manifest().ID, Manifest: pkg.Manifest(), Digest: pkg.Digest(), Enabled: true}, thirdParty))
	manager := NewManager(&Registry{}, &observedRuntime{}, WithStore(store))
	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{existing, added})))
	assert.Equal(t, 1, store.saves)
	assert.Equal(t, []string{"io.added", "io.thirdparty"}, store.reads)
	require.Len(t, manager.Plugins(), 3)
	require.NoError(t, manager.Close(ctx))
}

func TestFailedStartupClosesAllCandidatesWithoutPublication(t *testing.T) {
	ctx := context.Background()
	registry := &Registry{}
	runtime := &observedRuntime{fail: "io.b"}
	manager := NewManager(registry, runtime)
	require.ErrorContains(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{inventoryArchive(t, "io.a", "1.0.0", true, ""), inventoryArchive(t, "io.b", "1.0.0", true, "")})), "initialization failed")
	assert.Empty(t, registry.Snapshot().Entries)
	assert.Empty(t, manager.Plugins())
	require.Len(t, runtime.instances, 1)
	assert.True(t, runtime.instances[0].closed)
	require.NoError(t, manager.Close(ctx))
}

func TestReplacementFailuresKeepActiveInstallation(t *testing.T) {
	ctx := context.Background()
	id := "io.fixture"
	store := &observedStore{memoryStore: testStore(t, inventoryArchive(t, id, "1.0.0", true, ""), true)}
	runtime := &observedRuntime{}
	registry := &Registry{}
	manager := NewManager(registry, runtime, WithStore(store))
	require.NoError(t, manager.Bootstrap(ctx, nil))
	original := runtime.instances[0]
	candidate := inventoryArchive(t, id, "2.0.0", true, "")
	store.fail = true
	_, err := manager.Upgrade(ctx, id, candidate)
	require.ErrorContains(t, err, "persistence failed")
	assert.False(t, original.closed)
	assert.True(t, runtime.instances[1].closed)
	store.fail = false
	runtime.fail = id
	_, err = manager.Upgrade(ctx, id, candidate)
	require.ErrorContains(t, err, "initialization failed")
	assert.False(t, original.closed)
	assert.Equal(t, "1.0.0", manager.Plugins()[0].Manifest.Version)
	records, err := store.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", records[0].Manifest.Version)
	require.NoError(t, manager.Close(ctx))
}

func TestBootstrapRejectsStoredManifestThatDoesNotMatchPackage(t *testing.T) {
	ctx := context.Background()
	archive := inventoryArchive(t, "io.mismatch", "1.0.0", true, "")
	store := testStore(t, archive, true)
	record := store.records["io.mismatch"]
	record.Manifest.Description = "tampered metadata"
	store.records["io.mismatch"] = record

	manager := NewManager(&Registry{}, &observedRuntime{}, WithStore(store))
	err := manager.Bootstrap(ctx, nil)
	require.ErrorContains(t, err, "stored plugin package mismatch")
	assert.Empty(t, manager.Plugins())
	require.NoError(t, manager.Close(ctx))
}

// blockedSeedStore pauses after the initial inventory read to force a stale
// startup to race with a completed installation from another manager.
type blockedSeedStore struct {
	Store
	reached chan struct{}
	resume  chan struct{}
}

func (s *blockedSeedStore) SeedPlugin(ctx context.Context, r Record, archive []byte) error {
	close(s.reached)
	select {
	case <-s.resume:
		return s.Store.SeedPlugin(ctx, r, archive)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestConcurrentBootstrapUsesWinningInstallation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("winner_enabled_%t", enabled), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			const id = "io.concurrent"
			store := &memoryStore{records: make(map[string]Record)}
			blocked := &blockedSeedStore{Store: store, reached: make(chan struct{}), resume: make(chan struct{})}
			// The losing binary offers an older package with different dependencies
			// and the opposite default. None of those values may replace the winner.
			losingArchive := inventoryArchive(t, id, "1.0.0", !enabled, "io.unavailable")
			winningArchive := inventoryArchive(t, id, "2.0.0", enabled, "")
			loserRuntime, winnerRuntime := &observedRuntime{}, &observedRuntime{}
			loser := NewManager(&Registry{}, loserRuntime, WithStore(blocked))
			winner := NewManager(&Registry{}, winnerRuntime, WithStore(store))
			defer loser.Close(context.Background())  // nolint:errcheck
			defer winner.Close(context.Background()) // nolint:errcheck
			losingDistribution := testDistribution(t, [][]byte{losingArchive})
			done := make(chan error, 1)
			go func() { done <- loser.Bootstrap(ctx, losingDistribution) }()
			select {
			case <-blocked.reached:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			err := winner.Bootstrap(ctx, testDistribution(t, [][]byte{winningArchive}))
			close(blocked.resume)
			require.NoError(t, err)
			require.NoError(t, <-done)
			for _, manager := range []*Manager{winner, loser} {
				loaded := manager.Plugins()
				require.Len(t, loaded, 1)
				assert.Equal(t, "2.0.0", loaded[0].Manifest.Version)
				assert.Equal(t, enabled, loaded[0].Enabled)
				assert.Empty(t, loaded[0].Manifest.Requires)
			}
			if enabled {
				assert.Equal(t, []string{id}, loserRuntime.loads)
			} else {
				assert.Empty(t, loserRuntime.loads)
			}
			persisted, err := store.PluginPackage(ctx, id, packageDigest(t, winningArchive))
			require.NoError(t, err)
			assert.Equal(t, winningArchive, persisted)
		})
	}
}
