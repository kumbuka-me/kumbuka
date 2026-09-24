package plugin

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// settingsStorage provides test state for settings storage behavior.
type settingsStorage struct {
	// mu configures or records the mu value used by the fixture.
	mu sync.Mutex
	// values records the values observed by the test double.
	values map[string][]byte
	// writeErr configures the error returned by the test double.
	writeErr error
}

// ReadPluginValue reads one stored test value.
func (s *settingsStorage) ReadPluginValue(_ context.Context, id string, namespace StorageNamespace, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id+"/"+string(namespace)+"/"+key]
	return bytes.Clone(value), ok, nil
}

// ListPluginValues lists matching stored test values.
func (s *settingsStorage) ListPluginValues(_ context.Context, id string, namespace StorageNamespace, prefix string) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]byte)
	wanted := id + "/" + string(namespace) + "/" + prefix
	for key, value := range s.values {
		if strings.HasPrefix(key, wanted) {
			result[strings.TrimPrefix(key, id+"/"+string(namespace)+"/")] = bytes.Clone(value)
		}
	}
	return result, nil
}

// WritePluginValue stores one test plugin value.
func (s *settingsStorage) WritePluginValue(_ context.Context, id string, namespace StorageNamespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.writeErr != nil {
		return s.writeErr
	}
	s.values[id+"/"+string(namespace)+"/"+key] = bytes.Clone(value)
	return nil
}

// ReplacePluginValue atomically moves one test plugin value while rejecting collisions.
func (s *settingsStorage) ReplacePluginValue(_ context.Context, id string, namespace StorageNamespace, oldKey, newKey string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldPath := id + "/" + string(namespace) + "/" + oldKey
	newPath := id + "/" + string(namespace) + "/" + newKey
	if _, ok := s.values[oldPath]; !ok {
		return ErrPluginValueNotFound
	}
	if _, ok := s.values[newPath]; ok {
		return ErrPluginValueAlreadyExists
	}
	delete(s.values, oldPath)
	s.values[newPath] = bytes.Clone(value)
	return nil
}

// DeletePluginValue removes one test plugin value.
func (s *settingsStorage) DeletePluginValue(_ context.Context, id string, namespace StorageNamespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id+"/"+string(namespace)+"/"+key)
	return nil
}

// TestPluginSettingsDefaultAndPersistedState verifies feature defaults, dependencies, and persistence.
func TestPluginSettingsDefaultAndPersistedState(t *testing.T) {
	ctx := context.Background()
	storage := &settingsStorage{values: make(map[string][]byte)}
	manifest := pluginpackage.Manifest{
		ID: "io.example.settings",
		Modules: []pluginpackage.Module{
			{Type: "settings", ID: "colors", Name: "Colors"},
			{Type: "settings", ID: "filters", Name: "Filters", Requires: []string{"colors"}},
		},
	}
	manager := NewManager(&Registry{}, nil, WithStorage(storage))

	settings, err := manager.loadSettings(ctx, manifest)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"colors": true, "filters": true}, settings)

	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true, Settings: settings}}
	manager.order = []string{manifest.ID}

	require.ErrorContains(t, manager.UpdateSettings(ctx, manifest.ID, map[string]bool{"colors": false, "filters": true}), "requires")
	require.NoError(t, manager.UpdateSettings(ctx, manifest.ID, map[string]bool{"colors": false, "filters": false}))
	assert.Equal(t, map[string]bool{
		"io.example.settings.colors":  false,
		"io.example.settings.filters": false,
	}, manager.FeatureSettings())
	assert.Equal(t, []byte("false"), storage.values[manifest.ID+"/settings/feature:colors"])

	reloaded, err := manager.loadSettings(ctx, manifest)
	require.NoError(t, err)
	assert.Equal(t, map[string]bool{"colors": false, "filters": false}, reloaded)
}

// TestTypedPluginSettingsDefaultsPersistenceAndSecrets verifies singleton settings groups use defaults, validation, and encrypted storage.
func TestTypedPluginSettingsDefaultsPersistenceAndSecrets(t *testing.T) {
	ctx := context.Background()
	storage := &settingsStorage{values: make(map[string][]byte)}
	codec := resourceSecretCodec{configured: true}
	manifest := pluginpackage.Manifest{
		ID: "io.example.appearance",
		Modules: []pluginpackage.Module{{
			Type: "settings", ID: "appearance", Name: "Appearance", Fields: []pluginpackage.ConfigurationField{
				{ID: "position", Name: "Position", Type: "select", Required: true, Default: "right", Options: []string{"left", "right"}},
				{ID: "highlight", Name: "Highlight", Type: "boolean", Default: "true"},
				{ID: "token", Name: "Token", Type: "secret"},
			},
		}},
	}
	manager := NewManager(&Registry{}, nil, WithStorage(storage), WithSecretCodec(codec))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	groups, err := manager.SettingGroups(ctx, manifest.ID)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "right", groups[0].Values["position"])
	assert.Equal(t, "true", groups[0].Values["highlight"])
	assert.False(t, groups[0].SecretFields["token"])

	require.NoError(t, manager.SaveSettingGroup(ctx, manifest.ID, "appearance", map[string]string{
		"position": "left", "highlight": "false", "token": "secret",
	}))
	stored := storage.values[manifest.ID+"/settings/setting:appearance"]
	assert.Contains(t, string(stored), `"position":"left"`)
	assert.Contains(t, string(stored), `"token":"enc:secret"`)
	assert.NotContains(t, string(stored), `"token":"secret"`)

	groups, err = manager.SettingGroups(ctx, manifest.ID)
	require.NoError(t, err)
	assert.Equal(t, "left", groups[0].Values["position"])
	assert.Empty(t, groups[0].Values["token"])
	assert.True(t, groups[0].SecretFields["token"])

	require.NoError(t, manager.SaveSettingGroup(ctx, manifest.ID, "appearance", map[string]string{
		"position": "right", "highlight": "true", "token": "",
	}))
	value, declared, err := ReadDeclaredSetting(ctx, storage, manifest.ID, manifest, "appearance.token", codec)
	require.NoError(t, err)
	assert.True(t, declared)
	assert.Equal(t, "secret", string(value))

	value, declared, err = ReadDeclaredSetting(ctx, storage, manifest.ID, manifest, "appearance.position", codec)
	require.NoError(t, err)
	assert.True(t, declared)
	assert.Equal(t, "right", string(value))

	_, declared, err = ReadDeclaredSetting(ctx, storage, manifest.ID, manifest, "other.value", codec)
	require.NoError(t, err)
	assert.False(t, declared)

	err = manager.SaveSettingGroup(ctx, manifest.ID, "appearance", map[string]string{
		"position": "center", "highlight": "true", "token": "",
	})
	var fieldErr *ConfigurationFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, "position", fieldErr.Field)
}

func TestUpdateSettingsKeepsMemoryUnchangedWhenPersistenceFails(t *testing.T) {
	t.Parallel()

	failure := errors.New("settings unavailable")
	storage := &settingsStorage{values: make(map[string][]byte), writeErr: failure}
	manifest := pluginpackage.Manifest{
		ID: "io.example.settings",
		Modules: []pluginpackage.Module{
			{Type: "settings", ID: "colors", Name: "Colors"},
		},
	}
	manager := NewManager(&Registry{}, nil, WithStorage(storage))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{
		Manifest: manifest,
		Enabled:  true,
		Settings: map[string]bool{"colors": true},
	}}
	manager.order = []string{manifest.ID}

	err := manager.UpdateSettings(context.Background(), manifest.ID, map[string]bool{"colors": false})

	require.ErrorIs(t, err, failure)
	assert.Equal(t, map[string]bool{"colors": true}, manager.loaded[manifest.ID].metadata.Settings)
	assert.Empty(t, storage.values)
}
