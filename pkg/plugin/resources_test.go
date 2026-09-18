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

// resourceStorage provides isolated plugin-value persistence for resource tests.
type resourceStorage struct {
	mu     sync.Mutex
	values map[string][]byte
}

// ReadPluginValue reads one stored test value.
func (s *resourceStorage) ReadPluginValue(_ context.Context, id, namespace, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id+"/"+namespace+"/"+key]
	return bytes.Clone(value), ok, nil
}

// ListPluginValues lists matching stored test values.
func (s *resourceStorage) ListPluginValues(_ context.Context, id, namespace, prefix string) (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make(map[string][]byte)
	base := id + "/" + namespace + "/"
	for key, value := range s.values {
		if strings.HasPrefix(key, base+prefix) {
			result[strings.TrimPrefix(key, base)] = bytes.Clone(value)
		}
	}
	return result, nil
}

// WritePluginValue stores one test plugin value.
func (s *resourceStorage) WritePluginValue(_ context.Context, id, namespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id+"/"+namespace+"/"+key] = bytes.Clone(value)
	return nil
}

// ReplacePluginValue atomically moves one test plugin value while rejecting collisions.
func (s *resourceStorage) ReplacePluginValue(_ context.Context, id, namespace, oldKey, newKey string, value []byte) error {
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
func (s *resourceStorage) DeletePluginValue(_ context.Context, id, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id+"/"+namespace+"/"+key)
	return nil
}

// resourceSecretCodec provides deterministic reversible test encryption.
type resourceSecretCodec struct{ configured bool }

// Configured reports whether test encryption is enabled.
func (c resourceSecretCodec) Configured() bool { return c.configured }

// Encrypt protects a test value with a deterministic marker.
func (c resourceSecretCodec) Encrypt(value string) (string, error) {
	if !c.configured {
		return "", errors.New("not configured")
	}
	return "enc:" + value, nil
}

// Decrypt reveals a deterministic test value.
func (c resourceSecretCodec) Decrypt(value string) (string, error) {
	if !c.configured {
		return "", errors.New("not configured")
	}
	plain, ok := strings.CutPrefix(value, "enc:")
	if !ok {
		return "", errors.New("invalid ciphertext")
	}
	return plain, nil
}

// TestPluginResourcesAndEditorContributions verifies generic resources continue to feed declarative editor modules.
func TestPluginResourcesAndEditorContributions(t *testing.T) {
	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	manifest := pluginpackage.Manifest{
		ID: "io.example.resources",
		Modules: []pluginpackage.Module{
			{Type: "admin-resource", ID: "values", Name: "Values", Fields: []pluginpackage.ResourceField{
				{ID: "name", Name: "Name", Type: "text", Required: true, Key: true},
				{ID: "content", Name: "Value", Type: "textarea", Required: true},
			}},
			{Type: "editor-completion", ID: "values", Resource: "values", Trigger: "{{", Replacement: "{{var:${name}}}", LabelField: "name"},
			{Type: "editor-insert", ID: "value", Name: "Variable", Markdown: "{{"},
			{Type: "editor-insert", ID: "strike", Name: "Strikethrough", Markdown: "~~", Suffix: "~~", Placeholder: "text", Mode: "wrap", Group: "text", Icon: "strikethrough-lucide"},
		},
	}
	manager := NewManager(&Registry{}, nil, WithStorage(storage))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "values", "", map[string]string{"name": "Environment", "content": "production"}))
	records, err := manager.ResourceRecords(ctx, manifest.ID, "values")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "Environment", records[0].Key)
	assert.Equal(t, "production", records[0].Values["content"])

	completions, err := manager.EditorCompletions(ctx)
	require.NoError(t, err)
	require.Len(t, completions, 1)
	assert.Equal(t, "{{var:Environment}}", completions[0].Replacement)
	assert.Equal(t, "{{", completions[0].Trigger)

	inserts := manager.EditorInserts()
	require.Len(t, inserts, 2)
	assert.Equal(t, "Variable", inserts[0].Name)
	assert.Equal(t, "insert", inserts[0].Mode)
	assert.Equal(t, "insert", inserts[0].Group)
	assert.Equal(t, "wrap", inserts[1].Mode)
	assert.Equal(t, "text", inserts[1].Group)
	assert.Equal(t, "~~", inserts[1].Suffix)
	assert.Equal(t, "strikethrough-lucide", inserts[1].Icon)

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "values", "Environment", map[string]string{"name": "Stage", "content": "staging"}))
	require.NoError(t, manager.DeleteResourceRecord(ctx, manifest.ID, "values", "Stage"))
	records, err = manager.ResourceRecords(ctx, manifest.ID, "values")
	require.NoError(t, err)
	assert.Empty(t, records)
}

// TestPluginResourceFieldTypesAndSecrets verifies typed validation, masked administration reads, and plugin secret decryption.
func TestPluginResourceFieldTypesAndSecrets(t *testing.T) {
	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	codec := resourceSecretCodec{configured: true}
	manifest := pluginpackage.Manifest{ID: "io.example.remote", Modules: []pluginpackage.Module{{
		Type: "admin-resource", ID: "sources", Name: "Sources", Fields: []pluginpackage.ResourceField{
			{ID: "name", Name: "Name", Type: "text", Required: true, Key: true},
			{ID: "endpoint", Name: "Endpoint", Type: "url", Required: true},
			{ID: "provider", Name: "Provider", Type: "select", Required: true, Options: []string{"github", "gitlab"}, Default: "github"},
			{ID: "enabled", Name: "Enabled", Type: "boolean", Default: "true"},
			{ID: "token", Name: "Token", Type: "secret"},
		},
	}}}
	manager := NewManager(&Registry{}, nil, WithStorage(storage), WithSecretCodec(codec))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "sources", "", map[string]string{
		"name": "docs", "endpoint": "https://git.example.test", "provider": "gitlab", "enabled": "true", "token": "secret",
	}))
	adminRecords, err := manager.ResourceRecords(ctx, manifest.ID, "sources")
	require.NoError(t, err)
	require.Len(t, adminRecords, 1)
	assert.Empty(t, adminRecords[0].Values["token"])
	assert.True(t, adminRecords[0].SecretFields["token"])

	raw, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "docs")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "enc:secret", raw.Values["token"])
	revealed, err := RevealResourceSecrets(raw, manifest.Modules[0], codec)
	require.NoError(t, err)
	assert.Equal(t, "secret", revealed.Values["token"])

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "sources", "docs", map[string]string{
		"name": "docs", "endpoint": "https://git.example.test", "provider": "github", "enabled": "false", "token": "",
	}))
	raw, found, err = ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "docs")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "enc:secret", raw.Values["token"], "blank edit must preserve the existing secret")
	assert.Equal(t, "github", raw.Values["provider"])
	assert.Equal(t, "false", raw.Values["enabled"])

	err = manager.SaveResourceRecord(ctx, manifest.ID, "sources", "docs", map[string]string{
		"name": "docs", "endpoint": "file:///tmp/docs", "provider": "github", "enabled": "true", "token": "",
	})
	require.ErrorContains(t, err, "absolute HTTP or HTTPS URL")
}

// TestPluginResourceRenamePreservesSourceOnCollision verifies a conflicting rename never deletes the original record.
func TestPluginResourceRenamePreservesSourceOnCollision(t *testing.T) {
	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	manifest := pluginpackage.Manifest{ID: "io.example.rename", Modules: []pluginpackage.Module{{
		Type: "admin-resource", ID: "values", Name: "Values", Fields: []pluginpackage.ResourceField{
			{ID: "name", Name: "Name", Type: "text", Required: true, Key: true},
			{ID: "content", Name: "Content", Type: "text", Required: true},
		},
	}}}
	manager := NewManager(&Registry{}, nil, WithStorage(storage))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "values", "", map[string]string{"name": "source", "content": "one"}))
	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "values", "", map[string]string{"name": "target", "content": "two"}))

	err := manager.SaveResourceRecord(ctx, manifest.ID, "values", "source", map[string]string{"name": "target", "content": "changed"})
	require.ErrorContains(t, err, "already exists")

	source, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "source")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "one", source.Values["content"])

	target, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "target")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "two", target.Values["content"])
}
