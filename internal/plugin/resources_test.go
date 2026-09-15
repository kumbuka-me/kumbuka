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

type resourceStorage struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (s *resourceStorage) ReadPluginValue(_ context.Context, id, namespace, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[id+"/"+namespace+"/"+key]
	return bytes.Clone(value), ok, nil
}

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

func (s *resourceStorage) WritePluginValue(_ context.Context, id, namespace, key string, value []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[id+"/"+namespace+"/"+key] = bytes.Clone(value)
	return nil
}

func (s *resourceStorage) DeletePluginValue(_ context.Context, id, namespace, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.values, id+"/"+namespace+"/"+key)
	return nil
}

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
