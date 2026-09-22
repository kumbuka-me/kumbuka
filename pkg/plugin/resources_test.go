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
			{Type: "admin-resource", ID: "values", Name: "Values", Fields: []pluginpackage.ConfigurationField{
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

	providers := manager.EditorCompletionProviders()
	require.Len(t, providers, 1)
	assert.Equal(t, manifest.ID, providers[0].PluginID)
	assert.Equal(t, "values", providers[0].ResourceID)
	assert.Equal(t, "Values", providers[0].ResourceName)
	assert.Equal(t, "{{var:${name}}}", providers[0].Replacement)
	assert.Equal(t, "name", providers[0].LabelField)
	assert.True(t, providers[0].CanCreate)
	require.Len(t, providers[0].Fields, 2)
	assert.Equal(t, "name", providers[0].Fields[0].ID)
	assert.True(t, providers[0].Fields[0].Key)
	assert.Equal(t, "textarea", providers[0].Fields[1].Type)

	inserts := manager.EditorInserts()
	require.Len(t, inserts, 2)
	assert.Equal(t, "Variable", inserts[0].Name)
	assert.Equal(t, "insert", inserts[0].Mode)
	assert.Equal(t, "insert", inserts[0].Group)
	assert.Equal(t, "values", inserts[0].CompletionModuleID)
	assert.Equal(t, "wrap", inserts[1].Mode)
	assert.Equal(t, "text", inserts[1].Group)
	assert.Equal(t, "~~", inserts[1].Suffix)
	assert.Equal(t, "strikethrough-lucide", inserts[1].Icon)
	assert.Empty(t, inserts[1].CompletionModuleID)

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "values", "Environment", map[string]string{"name": "Stage", "content": "staging"}))
	require.NoError(t, manager.DeleteResourceRecord(ctx, manifest.ID, "values", "Stage"))
	records, err = manager.ResourceRecords(ctx, manifest.ID, "values")
	require.NoError(t, err)
	assert.Empty(t, records)
}

// TestPluginConfigurationFieldTypesAndSecrets verifies typed validation, masked administration reads, and plugin secret decryption.
func TestPluginConfigurationFieldTypesAndSecrets(t *testing.T) {
	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	codec := resourceSecretCodec{configured: true}
	manifest := pluginpackage.Manifest{ID: "io.example.remote", Modules: []pluginpackage.Module{{
		Type: "admin-resource", ID: "sources", Name: "Sources", Fields: []pluginpackage.ConfigurationField{
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
		Type: "admin-resource", ID: "values", Name: "Values", Fields: []pluginpackage.ConfigurationField{
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
	var fieldErr *ConfigurationFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, "name", fieldErr.Field)
	assert.Equal(t, "Name is already in use.", fieldErr.Message)

	source, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "source")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "one", source.Values["content"])

	target, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "target")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "two", target.Values["content"])
}

// TestPluginResourceValidationErrorsExposeFieldIDs verifies handlers can map resource validation back to manifest fields.
func TestPluginResourceValidationErrorsExposeFieldIDs(t *testing.T) {
	t.Parallel()

	field := pluginpackage.ConfigurationField{ID: "endpoint", Name: "API endpoint", Type: "url", Required: true}
	_, err := normalizeConfigurationValue(field, "file:///tmp/repository")

	var fieldErr *ConfigurationFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, "endpoint", fieldErr.Field)
	assert.Equal(t, "API endpoint must be an absolute HTTP or HTTPS URL.", fieldErr.Message)
}

// TestMaskResourceSecretsDoesNotMutateSource verifies presentation masking works on a defensive copy.
func TestMaskResourceSecretsDoesNotMutateSource(t *testing.T) {
	t.Parallel()

	module := pluginpackage.Module{Fields: []pluginpackage.ConfigurationField{
		{ID: "name", Name: "Name", Type: "text"},
		{ID: "token", Name: "Access token", Type: "secret"},
	}}
	record := ResourceRecord{Key: "source", Values: map[string]string{
		"name":  "source",
		"token": "encrypted-token",
	}}

	masked := MaskResourceSecrets(record, module)

	assert.Equal(t, "encrypted-token", record.Values["token"])
	assert.Empty(t, masked.Values["token"])
	assert.True(t, masked.SecretFields["token"])
}

// TestPluginResourceSecretRequiresConfiguredEncryption verifies plaintext secrets are never stored without encryption.
func TestPluginResourceSecretRequiresConfiguredEncryption(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	manifest := pluginpackage.Manifest{ID: "io.example.secret", Modules: []pluginpackage.Module{{
		Type: "admin-resource", ID: "sources", Name: "Sources", Fields: []pluginpackage.ConfigurationField{
			{ID: "name", Name: "Name", Type: "text", Required: true, Key: true},
			{ID: "token", Name: "Access token", Type: "secret"},
		},
	}}}
	manager := NewManager(&Registry{}, nil, WithStorage(storage), WithSecretCodec(resourceSecretCodec{}))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	err := manager.SaveResourceRecord(ctx, manifest.ID, "sources", "", map[string]string{
		"name": "private", "token": "secret",
	})

	assert.ErrorIs(t, err, ErrSecretEncryptionUnavailable)
	assert.Empty(t, storage.values)
}

// TestPluginResourceListAndColorFields verifies repeatable rows are normalized and validated before persistence.
func TestPluginResourceListAndColorFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	storage := &resourceStorage{values: make(map[string][]byte)}
	manifest := pluginpackage.Manifest{ID: "io.example.statuses", Modules: []pluginpackage.Module{{
		Type: "admin-resource", ID: "sets", Name: "Sets", Fields: []pluginpackage.ConfigurationField{
			{ID: "name", Name: "Name", Type: "text", Required: true, Key: true},
			{
				ID: "statuses", Name: "Statuses", Type: "list", Required: true, MaxItems: 4,
				Columns: []pluginpackage.ConfigurationField{
					{ID: "label", Name: "Status", Type: "text", Required: true, MaxBytes: 64},
					{ID: "color", Name: "Color", Type: "color", Required: true, Default: "#64748b"},
				},
			},
		},
	}}}
	manager := NewManager(&Registry{}, nil, WithStorage(storage))
	manager.loaded[manifest.ID] = managedPlugin{metadata: LoadedPlugin{Manifest: manifest, Enabled: true}}
	manager.order = []string{manifest.ID}

	require.NoError(t, manager.SaveResourceRecord(ctx, manifest.ID, "sets", "", map[string]string{
		"name":     "risk",
		"statuses": `[{"label":"Low","color":"#64748B"},{"label":"High","color":"#DC2626"}]`,
	}))
	record, found, err := ReadResourceRecord(ctx, storage, manifest.ID, manifest.Modules[0], "risk")
	require.NoError(t, err)
	require.True(t, found)
	assert.JSONEq(t, `[{"label":"Low","color":"#64748b"},{"label":"High","color":"#dc2626"}]`, record.Values["statuses"])

	err = manager.SaveResourceRecord(ctx, manifest.ID, "sets", "risk", map[string]string{
		"name":     "risk",
		"statuses": `[{"label":"Low","color":"rosa"}]`,
	})
	var fieldErr *ConfigurationFieldError
	require.ErrorAs(t, err, &fieldErr)
	assert.Equal(t, "statuses", fieldErr.Field)
	assert.Contains(t, fieldErr.Message, "row 1")
	assert.Contains(t, fieldErr.Message, "six-digit hex color")
}
