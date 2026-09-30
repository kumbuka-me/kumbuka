package plugin_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRuntime groups the state and data associated with fake runtime.
type fakeRuntime struct {
	// instance owns the active executable plugin instance.
	instance *fakeInstance
	// err stores the terminal operation error.
	err error
	// closed indicates whether the associated resource is closed.
	closed bool
}

// Load loads the value.
func (r *fakeRuntime) Load(context.Context, *pluginpackage.Package) (plugin.Instance, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.instance, nil
}

// Close releases resources held by the receiver.
func (r *fakeRuntime) Close(context.Context) error { r.closed = true; return nil }

// fakeInstance groups the state and data associated with fake instance.
type fakeInstance struct {
	// closed indicates whether the associated resource is closed.
	closed bool
}

// Contributions returns the contributions owned by the instance.
func (i *fakeInstance) Contributions() plugin.Contributions { return plugin.Contributions{} }

// Close releases resources held by the receiver.
func (i *fakeInstance) Close(context.Context) error { i.closed = true; return nil }

// TestManagerRollsBackFailedRegistration verifies manager rolls back failed registration behavior.
func TestManagerRollsBackFailedRegistration(t *testing.T) {
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "me.kumbuka.callouts", Name: "Existing"}, plugin.Contributions{}))
	runtime := &fakeRuntime{instance: &fakeInstance{}}
	manager := plugin.NewManager(registry, runtime)
	_, err = manager.Install(context.Background(), data)
	require.Error(t, err)
	assert.True(t, runtime.instance.closed)
	assert.Empty(t, manager.Plugins())
	assert.Equal(t, "Existing", registry.Snapshot().Entries[0].Descriptor.Name)
	require.NoError(t, manager.Close(context.Background()))
	assert.True(t, runtime.closed)
}

// TestManagerFailuresNeverPublishContributions verifies manager failures never publish contributions behavior.
func TestManagerFailuresNeverPublishContributions(t *testing.T) {
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	registry := &plugin.Registry{}
	runtime := &fakeRuntime{err: errors.New("invalid reactor")}
	manager := plugin.NewManager(registry, runtime)
	err = manager.Bootstrap(context.Background(), testDistribution(t, [][]byte{data}))
	require.ErrorContains(t, err, "invalid reactor")
	assert.Empty(t, registry.Snapshot().Entries)
	_, err = manager.Install(context.Background(), []byte("invalid package"))
	require.Error(t, err)
	assert.Empty(t, registry.Snapshot().Entries)
	require.NoError(t, manager.Close(context.Background()))
	_, err = manager.Install(context.Background(), data)
	require.ErrorContains(t, err, "closed")
}

// TestInstallCannotReplaceAnotherRegistryOwner verifies install cannot replace another registry owner behavior.
func TestInstallCannotReplaceAnotherRegistryOwner(t *testing.T) {
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "me.kumbuka.callouts", Name: "Existing"}, plugin.Contributions{}))
	runtime := &fakeRuntime{instance: &fakeInstance{}}
	manager := plugin.NewManager(registry, runtime)
	_, err = manager.Install(context.Background(), data)
	require.ErrorContains(t, err, "already registered")
	assert.True(t, runtime.instance.closed)
	assert.Empty(t, manager.Plugins())
	assert.Equal(t, "Existing", registry.Snapshot().Entries[0].Descriptor.Name)
	require.NoError(t, manager.Close(context.Background()))
}

func TestRequiredPluginsAreOperatorPolicy(t *testing.T) {
	ctx := context.Background()
	archive, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	runtime := &fakeRuntime{instance: &fakeInstance{}}
	manager := plugin.NewManager(&plugin.Registry{}, runtime, plugin.WithRequiredPlugins("me.kumbuka.callouts"))
	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{archive})))
	assert.True(t, manager.IsRequired("me.kumbuka.callouts"))
	require.Error(t, manager.Disable(ctx, "me.kumbuka.callouts"))
	require.Error(t, manager.Uninstall(ctx, "me.kumbuka.callouts"))
	assert.True(t, manager.Plugins()[0].Enabled)
	require.NoError(t, manager.Close(ctx))
}

// requiredPolicyStore provides persisted disabled state for required-plugin bootstrap tests.
type requiredPolicyStore struct {
	archive     []byte
	enabled     bool
	stateWrites int
}

func (s *requiredPolicyStore) ListPlugins(context.Context) ([]plugin.Record, error) {
	pkg, err := pluginpackage.Read(s.archive)
	if err != nil {
		return nil, err
	}
	return []plugin.Record{{ID: pkg.Manifest().ID, Manifest: pkg.Manifest(), Digest: pkg.Digest(), README: pkg.README(), Enabled: s.enabled}}, nil
}

func (*requiredPolicyStore) SavePlugin(context.Context, plugin.Record, []byte) error {
	return errors.New("unexpected package write")
}

func (*requiredPolicyStore) DeletePlugin(context.Context, string) error {
	return errors.New("unexpected policy delete")
}

func TestRequiredPolicyOverridesDisabledBootstrapRecord(t *testing.T) {
	ctx := context.Background()
	archive, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	store := &requiredPolicyStore{archive: archive}
	runtime := &fakeRuntime{instance: &fakeInstance{}}
	manager := plugin.NewManager(&plugin.Registry{}, runtime, plugin.WithStore(store), plugin.WithRequiredPlugins("me.kumbuka.callouts"))
	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{archive})))
	assert.True(t, manager.Plugins()[0].Enabled)
	assert.True(t, store.enabled)
	assert.Equal(t, 1, store.stateWrites)
	require.NoError(t, manager.Close(ctx))

	runtime = &fakeRuntime{instance: &fakeInstance{}}
	manager = plugin.NewManager(&plugin.Registry{}, runtime, plugin.WithStore(store), plugin.WithRequiredPlugins("me.kumbuka.callouts"))
	require.NoError(t, manager.Bootstrap(ctx, testDistribution(t, [][]byte{archive})))
	assert.True(t, manager.Plugins()[0].Enabled)
	assert.Equal(t, 1, store.stateWrites, "persisted required state should not be rewritten on restart")
	require.NoError(t, manager.Close(ctx))
	missing := plugin.NewManager(&plugin.Registry{}, &fakeRuntime{}, plugin.WithRequiredPlugins("missing"))
	require.ErrorContains(t, missing.Bootstrap(ctx, nil), "required plugin missing is missing")
	require.NoError(t, missing.Close(ctx))
}

func (s *requiredPolicyStore) PluginPackage(context.Context, string) ([]byte, error) {
	return s.archive, nil
}

func (s *requiredPolicyStore) SetPluginEnabled(_ context.Context, _ string, enabled bool) error {
	s.enabled = enabled
	s.stateWrites++
	return nil
}
