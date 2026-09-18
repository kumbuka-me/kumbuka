package plugin

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type settingsStorage struct {
	mu     sync.Mutex
	values map[string][]byte
}

// ReadPluginValue reads one stored test value.
func (s *settingsStorage) ReadPluginValue(_ context.Context, id, namespace, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id+"/"+namespace+"/"+key]
	return bytes.Clone(value), ok, nil
}

// ListPluginValues lists matching stored test values.
func (s *settingsStorage) ListPluginValues(_ context.Context, id, namespace, prefix string) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]byte)
	wanted := id + "/" + namespace + "/" + prefix
	for key, value := range s.values {
		if strings.HasPrefix(key, wanted) {
			result[strings.TrimPrefix(key, id+"/"+namespace+"/")] = bytes.Clone(value)
		}
	}
	return result, nil
}

// WritePluginValue stores one test plugin value.
func (s *settingsStorage) WritePluginValue(_ context.Context, id, namespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id+"/"+namespace+"/"+key] = bytes.Clone(value)
	return nil
}

// ReplacePluginValue atomically moves one test plugin value while rejecting collisions.
func (s *settingsStorage) ReplacePluginValue(_ context.Context, id, namespace, oldKey, newKey string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldPath := id + "/" + namespace + "/" + oldKey
	newPath := id + "/" + namespace + "/" + newKey
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
func (s *settingsStorage) DeletePluginValue(_ context.Context, id, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id+"/"+namespace+"/"+key)
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
