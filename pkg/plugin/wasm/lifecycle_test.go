package wasm_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// installationStore groups the state and data associated with installation store.
type installationStore struct {
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// records indexes the state associated with records.
	records map[string]plugin.Record
	// fail stores the value associated with fail.
	fail bool
}

// ListPlugins lists plugins.
func (s *installationStore) ListPlugins(context.Context) ([]plugin.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []plugin.Record
	for _, r := range s.records {
		r.Package = bytes.Clone(r.Package)
		result = append(result, r)
	}
	return result, nil
}

// SavePlugin saves plugin.
func (s *installationStore) SavePlugin(_ context.Context, r plugin.Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("storage failed")
	}
	r.Package = bytes.Clone(r.Package)
	s.records[r.ID] = r
	return nil
}

// DeletePlugin deletes plugin.
func (s *installationStore) DeletePlugin(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("storage failed")
	}
	delete(s.records, id)
	return nil
}

// changedManifest handles the changed manifest operation.
func changedManifest(t *testing.T, data []byte, change func(string) string) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, file := range reader.File {
		r, err := file.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(r)
		require.NoError(t, err)
		require.NoError(t, r.Close())
		if file.Name == "plugin.yaml" {
			content = []byte(change(string(content)))
		}
		w, err := writer.Create(file.Name)
		require.NoError(t, err)
		_, err = w.Write(content)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return buffer.Bytes()
}

// changedManifestVersion returns a copy of data with only the manifest version changed.
func changedManifestVersion(t *testing.T, data []byte, version string) []byte {
	t.Helper()
	pkg, err := pluginpackage.Read(data)
	require.NoError(t, err)
	current := pkg.Manifest().Version
	return changedManifest(t, data, func(manifest string) string {
		from := "version: " + current
		require.Contains(t, manifest, from)
		return strings.Replace(manifest, from, "version: "+version, 1)
	})
}

// lifecycleManager handles the lifecycle manager operation.
func lifecycleManager(t *testing.T, store plugin.Store) (*plugin.Manager, *markdown.Renderer, *plugin.Registry) {
	t.Helper()
	return lifecycleManagerWithOptions(t, store)
}

func lifecycleManagerWithOptions(t *testing.T, store plugin.Store, options ...wasm.Option) (*plugin.Manager, *markdown.Renderer, *plugin.Registry) {
	t.Helper()
	runtime, err := wasm.New(context.Background(), wasm.Limits{}, options...)
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime, plugin.WithStore(store))
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })
	return manager, markdown.NewWithRegistry(registry), registry
}

// TestRuntimeLifecycleWithoutRestart verifies runtime lifecycle without restart behavior.
func TestRuntimeLifecycleWithoutRestart(t *testing.T) {
	ctx := context.Background()
	store := &installationStore{records: make(map[string]plugin.Record)}
	manager, renderer, _ := lifecycleManager(t, store)
	callouts, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	callouts = changedManifest(t, callouts, func(s string) string { return strings.ReplaceAll(s, "me.kumbuka.callouts", "io.example.lifecycle") })
	render := func() string {
		html, err := renderer.Render("!!! note\nMessage\n")
		require.NoError(t, err)
		return html
	}
	assert.NotContains(t, render(), `class="callout`)
	store.fail = true
	_, err = manager.Install(ctx, callouts)
	require.ErrorContains(t, err, "storage failed")
	assert.Empty(t, manager.Plugins())
	assert.Empty(t, store.records)
	store.fail = false
	metadata, err := manager.Install(ctx, callouts)
	require.NoError(t, err)
	assert.True(t, metadata.Enabled)
	assert.Contains(t, render(), `class="callout note"`)
	require.NoError(t, manager.Disable(ctx, metadata.Manifest.ID))
	assert.NotContains(t, render(), `class="callout`)
	assert.False(t, manager.Plugins()[0].Enabled)
	require.NoError(t, manager.Enable(ctx, metadata.Manifest.ID))
	assert.Contains(t, render(), `class="callout note"`)
	// A different actual reactor stops transforming callouts after upgrade.
	binary, err := fixtureWASM()
	require.NoError(t, err)
	next := changedManifest(t, archive(t, binary, "preprocess"), func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, "io.example.fixture", "io.example.lifecycle"), "1.0.0", "2.0.0")
	})
	store.fail = true
	_, err = manager.Upgrade(ctx, metadata.Manifest.ID, next)
	require.ErrorContains(t, err, "storage failed")
	assert.Contains(t, render(), `class="callout note"`)
	assert.Equal(t, metadata.Manifest.Version, manager.Plugins()[0].Manifest.Version)
	require.ErrorContains(t, manager.Disable(ctx, metadata.Manifest.ID), "storage failed")
	require.ErrorContains(t, manager.Uninstall(ctx, metadata.Manifest.ID), "storage failed")
	store.fail = false
	_, err = manager.Upgrade(ctx, metadata.Manifest.ID, next)
	require.NoError(t, err)
	assert.NotContains(t, render(), `class="callout`)
	assert.Equal(t, "2.0.0", manager.Plugins()[0].Manifest.Version)
	require.NoError(t, manager.Disable(ctx, metadata.Manifest.ID))
	restored, restoredRenderer, _ := lifecycleManager(t, store)
	require.NoError(t, restored.Bootstrap(ctx, nil))
	assert.False(t, restored.Plugins()[0].Enabled)
	require.NoError(t, restored.Enable(ctx, metadata.Manifest.ID))
	html, err := restoredRenderer.Render("Plain")
	require.NoError(t, err)
	assert.Contains(t, html, "Plain")
	require.NoError(t, restored.Uninstall(ctx, metadata.Manifest.ID))
	assert.Empty(t, restored.Plugins())
	records, err := store.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Empty(t, records)
}

// TestUpgradeRetainsAcquiredVersionUntilRenderFinishes verifies upgrade retains acquired version until render finishes behavior.
func TestUpgradeRetainsAcquiredVersionUntilRenderFinishes(t *testing.T) {
	ctx := context.Background()
	store := &installationStore{records: make(map[string]plugin.Record)}
	manager, renderer, registry := lifecycleManager(t, store)
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	metadata, err := manager.Install(ctx, data)
	require.NoError(t, err)
	plan, release := registry.AcquireRenderPlan()
	defer release()
	next := changedManifestVersion(t, data, "2.0.0")
	_, err = manager.Upgrade(ctx, metadata.Manifest.ID, next)
	require.NoError(t, err)
	source := "Ordinary text"
	got, err := plan.Preprocessors[0].Module.Preprocess(plugin.Context{}, source)
	require.NoError(t, err)
	assert.Contains(t, got, source)
	current, err := renderer.Render(source)
	require.NoError(t, err)
	assert.Contains(t, current, source)
	// Close respects outstanding render leases and can be retried after release.
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, manager.Close(cancelled), context.Canceled)
	release()
	require.NoError(t, manager.Close(ctx))
}

// TestBundledDisableAndInstalledOverrideSurviveRestore verifies bundled disable and installed override survive restore behavior.
func TestBundledDisableAndInstalledOverrideSurviveRestore(t *testing.T) {
	ctx := context.Background()
	store := &installationStore{records: make(map[string]plugin.Record)}
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	manager, _, _ := lifecycleManager(t, store)
	require.NoError(t, manager.Bootstrap(ctx, [][]byte{data}))
	id := "me.kumbuka.callouts"
	require.NoError(t, manager.Disable(ctx, id))
	restored, _, _ := lifecycleManager(t, store)
	require.NoError(t, restored.Bootstrap(ctx, [][]byte{data}))
	assert.False(t, restored.Plugins()[0].Enabled)
	metadata, err := restored.Upgrade(ctx, id, data)
	require.NoError(t, err)
	assert.Equal(t, plugin.SourceInstalled, metadata.Source)
	assert.False(t, metadata.Enabled)
	require.NoError(t, restored.Enable(ctx, id))
	require.NoError(t, restored.Uninstall(ctx, id))
	assert.Equal(t, plugin.SourceBundled, restored.Plugins()[0].Source)
	assert.False(t, restored.Plugins()[0].Enabled)
	final, _, _ := lifecycleManager(t, store)
	require.NoError(t, final.Bootstrap(ctx, [][]byte{data}))
	assert.False(t, final.Plugins()[0].Enabled)
	records, err := store.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Empty(t, records[0].Package)
}

// blockingPreprocessor groups the state and data associated with blocking preprocessor.
type blockingPreprocessor struct {
	// start coordinates the state associated with start and proceed.
	start, proceed chan struct{}
	// once stores the value associated with once.
	once sync.Once
}

// Preprocess transforms Markdown before the core parser runs.
func (p *blockingPreprocessor) Preprocess(_ plugin.Context, source string) (string, error) {
	p.once.Do(func() { close(p.start); <-p.proceed })
	return source, nil
}

// TestUpgradeDuringRenderingKeepsWholeSnapshotAlive verifies upgrade during rendering keeps whole snapshot alive behavior.
func TestUpgradeDuringRenderingKeepsWholeSnapshotAlive(t *testing.T) {
	ctx := context.Background()
	store := &installationStore{records: make(map[string]plugin.Record)}
	manager, renderer, registry := lifecycleManagerWithOptions(t, store, wasm.WithInterpreter())
	blocker := &blockingPreprocessor{start: make(chan struct{}), proceed: make(chan struct{})}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "blocker", Name: "Blocker"}, plugin.Contributions{Preprocessors: []plugin.Preprocessor{blocker}}))
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	_, err = manager.Install(ctx, data)
	require.NoError(t, err)
	result := make(chan error, 1)
	go func() {
		html, err := renderer.Render("!!! note\nIn flight")
		if err == nil && !strings.Contains(html, `class="callout note"`) {
			err = errors.New("old snapshot lost its callout")
		}
		result <- err
	}()
	<-blocker.start
	next := changedManifestVersion(t, data, "2.0.0")
	_, err = manager.Upgrade(ctx, "me.kumbuka.callouts", next)
	close(blocker.proceed)
	require.NoError(t, err)
	require.NoError(t, <-result)
	assert.Equal(t, "2.0.0", manager.Plugins()[0].Manifest.Version)
}

// TestLifecycleDependenciesAndFailedBootstrapAreAtomic verifies lifecycle dependencies and failed bootstrap are atomic behavior.
func TestLifecycleDependenciesAndFailedBootstrapAreAtomic(t *testing.T) {
	ctx := context.Background()
	store := &installationStore{records: make(map[string]plugin.Record)}
	manager, _, registry := lifecycleManager(t, store)
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	base := changedManifest(t, data, func(s string) string { return strings.ReplaceAll(s, "me.kumbuka.callouts", "io.base") })
	child := changedManifest(t, data, func(s string) string {
		return strings.ReplaceAll(s, "me.kumbuka.callouts", "io.child") + "\nrequires: [io.base]\n"
	})
	_, err = manager.Install(ctx, child)
	require.ErrorContains(t, err, "requires")
	assert.Empty(t, registry.Snapshot().Entries)
	assert.Empty(t, store.records)
	_, err = manager.Install(ctx, base)
	require.NoError(t, err)
	_, err = manager.Install(ctx, child)
	require.NoError(t, err)
	require.ErrorContains(t, manager.Disable(ctx, "io.base"), "requires")
	require.ErrorContains(t, manager.Uninstall(ctx, "io.base"), "requires")
	cycle := changedManifest(t, base, func(s string) string { return s + "\nrequires: [io.child]\n" })
	_, err = manager.Upgrade(ctx, "io.base", cycle)
	require.ErrorContains(t, err, "cycle")
	require.Len(t, registry.Snapshot().Entries, 2)
	assert.True(t, store.records["io.base"].Enabled)
	_, err = manager.Upgrade(ctx, "io.base", []byte("broken"))
	require.Error(t, err)
	require.NoError(t, manager.Disable(ctx, "io.child"))
	require.NoError(t, manager.Disable(ctx, "io.base"))
	require.ErrorContains(t, manager.Enable(ctx, "io.child"), "requires")
	// A malformed durable enabled graph must never publish a partial startup.
	state := store.records["io.child"]
	state.Enabled = true
	store.records["io.child"] = state
	restored, _, restoredRegistry := lifecycleManager(t, store)
	require.ErrorContains(t, restored.Bootstrap(ctx, nil), "requires")
	assert.Empty(t, restoredRegistry.Snapshot().Entries)
	// A valid upgrade may change dependency order; shutdown must detach all
	// owned entries together rather than trusting historical installation order.
	independent := changedManifest(t, child, func(s string) string { return strings.ReplaceAll(s, "requires: [io.base]", "") })
	_, err = manager.Upgrade(ctx, "io.child", independent)
	require.NoError(t, err)
	require.NoError(t, manager.Enable(ctx, "io.child"))
	require.NoError(t, manager.Enable(ctx, "io.base"))
	_, err = manager.Upgrade(ctx, "io.base", cycle)
	require.NoError(t, err)
	require.NoError(t, manager.Close(ctx))
	assert.Empty(t, registry.Snapshot().Entries)

}
