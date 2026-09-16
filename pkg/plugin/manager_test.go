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
	err = manager.Bootstrap(context.Background(), [][]byte{data})
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
	require.NoError(t, manager.Bootstrap(ctx, [][]byte{archive}))
	assert.True(t, manager.IsRequired("me.kumbuka.callouts"))
	require.Error(t, manager.Disable(ctx, "me.kumbuka.callouts"))
	require.Error(t, manager.Uninstall(ctx, "me.kumbuka.callouts"))
	assert.True(t, manager.Plugins()[0].Enabled)
	require.NoError(t, manager.Close(ctx))
}

type disabledPolicyStore struct{}

func (disabledPolicyStore) ListPlugins(context.Context) ([]plugin.Record, error) {
	return []plugin.Record{{ID: "me.kumbuka.callouts", Source: plugin.SourceBundled, Enabled: false}}, nil
}
func (disabledPolicyStore) SavePlugin(context.Context, plugin.Record) error {
	return errors.New("unexpected policy write")
}
func (disabledPolicyStore) DeletePlugin(context.Context, string) error {
	return errors.New("unexpected policy delete")
}

func TestRequiredPolicyOverridesDisabledBootstrapRecord(t *testing.T) {
	ctx := context.Background()
	archive, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	runtime := &fakeRuntime{instance: &fakeInstance{}}
	manager := plugin.NewManager(&plugin.Registry{}, runtime, plugin.WithStore(disabledPolicyStore{}), plugin.WithRequiredPlugins("me.kumbuka.callouts"))
	require.NoError(t, manager.Bootstrap(ctx, [][]byte{archive}))
	assert.True(t, manager.Plugins()[0].Enabled)
	require.NoError(t, manager.Close(ctx))
	missing := plugin.NewManager(&plugin.Registry{}, &fakeRuntime{}, plugin.WithRequiredPlugins("missing"))
	require.ErrorContains(t, missing.Bootstrap(ctx, nil), "required plugin missing is missing")
	require.NoError(t, missing.Close(ctx))
}
