package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	resourceNamespace      = "data"
	maxResourceKeyBytes    = 128
	maxResourceRecordBytes = 60 << 10
)

// ParameterError reports invalid request-local plugin export input.
type ParameterError struct {
	PluginID string
	ModuleID string
	Key      string
	Message  string
}

// Error returns a safe validation message for one plugin export parameter.
func (e *ParameterError) Error() string {
	if e.Key == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Key, e.Message)
}

// ResourceRecord contains one persisted record owned by an admin-resource module.
type ResourceRecord struct {
	// Key is the canonical value of the resource's unique key field.
	Key string
	// Values contains validated field values keyed by manifest field ID.
	Values map[string]string
}

// EditorCompletionItem is one concrete resource-backed editor completion.
type EditorCompletionItem struct {
	// PluginID and ModuleID identify the owning completion contribution.
	PluginID string `json:"plugin_id"`
	ModuleID string `json:"module_id"`
	// Trigger opens completion when typed immediately before the query.
	Trigger string `json:"trigger"`
	// Label and Detail are displayed by editor completion UI.
	Label  string `json:"label"`
	Detail string `json:"detail,omitempty"`
	// Replacement is inserted when the item is selected.
	Replacement string `json:"replacement"`
}

// EditorInsertContribution is one active declarative editor action.
type EditorInsertContribution struct {
	// PluginID and ModuleID identify the owning contribution.
	PluginID string `json:"plugin_id"`
	ModuleID string `json:"module_id"`
	// Name and Description are displayed by editor insertion UI.
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Markdown and Suffix describe inserted, wrapped, or line-prefixed source.
	Markdown string `json:"markdown"`
	Suffix   string `json:"suffix,omitempty"`
	// Placeholder supplies default selected text for wrap and prefix actions.
	Placeholder string `json:"placeholder,omitempty"`
	// Mode and Group select generic editor behavior and toolbar placement.
	Mode  string `json:"mode"`
	Group string `json:"group"`
	// Icon is the optional host icon shown for the action.
	Icon string `json:"icon,omitempty"`
	// Inline reports whether plain insertion should avoid surrounding line breaks.
	Inline bool `json:"inline"`
}

// ResourceRecords returns all records for one declared admin resource.
func (m *Manager) ResourceRecords(ctx context.Context, pluginID, moduleID string) ([]ResourceRecord, error) {
	module, err := m.resourceModule(pluginID, moduleID)
	if err != nil {
		return nil, err
	}
	return ReadResourceRecords(ctx, m.values, pluginID, module)
}

// ReadResourceRecords decodes one resource collection from trusted plugin storage.
func ReadResourceRecords(ctx context.Context, storage Storage, pluginID string, module pluginpackage.Module) ([]ResourceRecord, error) {
	if storage == nil {
		return nil, nil
	}
	prefix := resourcePrefix(module.ID)
	stored, err := storage.ListPluginValues(ctx, pluginID, resourceNamespace, prefix)
	if err != nil {
		return nil, err
	}
	records := make([]ResourceRecord, 0, len(stored))
	for _, data := range stored {
		var values map[string]string
		if err := json.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("decode plugin resource %s.%s: %w", pluginID, module.ID, err)
		}
		keyField := resourceKeyField(module)
		key := values[keyField.ID]
		if key == "" {
			return nil, fmt.Errorf("plugin resource %s.%s contains a record without its key field", pluginID, module.ID)
		}
		records = append(records, ResourceRecord{Key: key, Values: values})
	}
	sort.Slice(records, func(i, j int) bool {
		left, right := strings.ToLower(records[i].Key), strings.ToLower(records[j].Key)
		if left == right {
			return records[i].Key < records[j].Key
		}
		return left < right
	})
	return records, nil
}

// SaveResourceRecord creates or replaces one plugin-owned resource record.
func (m *Manager) SaveResourceRecord(ctx context.Context, pluginID, moduleID, originalKey string, values map[string]string) error {
	module, err := m.resourceModule(pluginID, moduleID)
	if err != nil {
		return err
	}
	if m.values == nil {
		return errors.New("plugin resource storage is unavailable")
	}
	validated, key, err := validateResourceRecord(module, values)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(validated)
	if err != nil {
		return err
	}
	if len(encoded) > maxResourceRecordBytes {
		return errors.New("plugin resource record is too large")
	}
	newStorageKey := resourceStorageKey(module.ID, key)
	if originalKey != "" {
		oldStorageKey := resourceStorageKey(module.ID, originalKey)
		if oldStorageKey != newStorageKey {
			if _, found, err := m.values.ReadPluginValue(ctx, pluginID, resourceNamespace, newStorageKey); err != nil {
				return err
			} else if found {
				return errors.New("a resource record with that key already exists")
			}
			if err := m.values.DeletePluginValue(ctx, pluginID, resourceNamespace, oldStorageKey); err != nil {
				return err
			}
		}
	}
	return m.values.WritePluginValue(ctx, pluginID, resourceNamespace, newStorageKey, encoded)
}

// DeleteResourceRecord deletes one plugin-owned resource record.
func (m *Manager) DeleteResourceRecord(ctx context.Context, pluginID, moduleID, key string) error {
	module, err := m.resourceModule(pluginID, moduleID)
	if err != nil {
		return err
	}
	if m.values == nil {
		return errors.New("plugin resource storage is unavailable")
	}
	key = strings.TrimSpace(key)
	if !validResourceKey(key) {
		return errors.New("invalid plugin resource key")
	}
	return m.values.DeletePluginValue(ctx, pluginID, resourceNamespace, resourceStorageKey(module.ID, key))
}

// EditorCompletions expands active resource-backed completion declarations into concrete items.
func (m *Manager) EditorCompletions(ctx context.Context) ([]EditorCompletionItem, error) {
	m.mu.Lock()
	plugins := make([]LoadedPlugin, 0, len(m.order))
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if ok && item.metadata.Enabled {
			plugins = append(plugins, cloneLoaded(item.metadata))
		}
	}
	m.mu.Unlock()

	var result []EditorCompletionItem
	for _, item := range plugins {
		for _, module := range item.Manifest.Modules {
			if module.Type != "editor-completion" {
				continue
			}
			records, err := m.ResourceRecords(ctx, item.Manifest.ID, module.Resource)
			if err != nil {
				return nil, err
			}
			for _, record := range records {
				result = append(result, EditorCompletionItem{
					PluginID:    item.Manifest.ID,
					ModuleID:    module.ID,
					Trigger:     module.Trigger,
					Label:       record.Values[module.LabelField],
					Detail:      record.Values[module.DetailField],
					Replacement: expandResourceTemplate(module.Replacement, record.Values),
				})
			}
		}
	}
	return result, nil
}

// EditorInserts returns active plugin-owned declarative editor actions.
func (m *Manager) EditorInserts() []EditorInsertContribution {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []EditorInsertContribution
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "editor-insert" {
				continue
			}
			mode := module.Mode
			if mode == "" {
				mode = "insert"
			}
			group := module.Group
			if group == "" {
				group = "insert"
			}
			result = append(result, EditorInsertContribution{
				PluginID:    id,
				ModuleID:    module.ID,
				Name:        module.Name,
				Description: module.Description,
				Markdown:    module.Markdown,
				Suffix:      module.Suffix,
				Placeholder: module.Placeholder,
				Mode:        mode,
				Group:       group,
				Icon:        module.Icon,
				Inline:      module.Inline,
			})
		}
	}
	return result
}

// resourceModule returns one declared admin resource from a loaded plugin.
func (m *Manager) resourceModule(pluginID, moduleID string) (pluginpackage.Module, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.loaded[pluginID]
	if !ok {
		return pluginpackage.Module{}, errors.New("plugin is not installed")
	}
	for _, module := range item.metadata.Manifest.Modules {
		if module.ID == moduleID && module.Type == "admin-resource" {
			return module, nil
		}
	}
	return pluginpackage.Module{}, errors.New("plugin resource is not declared")
}

// resourceKeyField returns the unique key field in a validated resource module.
func resourceKeyField(module pluginpackage.Module) pluginpackage.ResourceField {
	for _, field := range module.Fields {
		if field.Key {
			return field
		}
	}
	return pluginpackage.ResourceField{}
}

// validateResourceRecord validates and normalizes one record against its manifest schema.
func validateResourceRecord(module pluginpackage.Module, values map[string]string) (map[string]string, string, error) {
	declared := make(map[string]pluginpackage.ResourceField, len(module.Fields))
	for _, field := range module.Fields {
		declared[field.ID] = field
	}
	for field := range values {
		if _, ok := declared[field]; !ok {
			return nil, "", fmt.Errorf("unknown resource field %q", field)
		}
	}
	result := make(map[string]string, len(module.Fields))
	key := ""
	for _, field := range module.Fields {
		value := values[field.ID]
		if field.Type == "text" || field.Key {
			value = strings.TrimSpace(value)
		}
		if field.Required && value == "" {
			return nil, "", fmt.Errorf("%s is required", field.Name)
		}
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return nil, "", fmt.Errorf("%s must contain valid UTF-8 text", field.Name)
		}
		limit := field.MaxBytes
		if limit == 0 {
			if field.Key {
				limit = maxResourceKeyBytes
			} else if field.Type == "textarea" {
				limit = 48 << 10
			} else {
				limit = 4096
			}
		}
		if len(value) > limit {
			return nil, "", fmt.Errorf("%s is too long", field.Name)
		}
		if field.Key {
			if !validResourceKey(value) {
				return nil, "", fmt.Errorf("%s contains unsupported characters", field.Name)
			}
			key = value
		}
		result[field.ID] = value
	}
	return result, key, nil
}

// validResourceKey reports whether key can be used in a macro and plugin storage key.
func validResourceKey(key string) bool {
	return key != "" && len(key) <= maxResourceKeyBytes && utf8.ValidString(key) && strings.TrimSpace(key) == key && !strings.ContainsAny(key, "\x00\r\n{}")
}

// resourcePrefix returns the storage prefix for one resource module.
func resourcePrefix(moduleID string) string { return "r:" + moduleID + ":" }

// resourceStorageKey returns the case-insensitive storage key for one resource record.
func resourceStorageKey(moduleID, key string) string {
	return resourcePrefix(moduleID) + strings.ToLower(strings.TrimSpace(key))
}

// expandResourceTemplate replaces ${field} placeholders with record values.
func expandResourceTemplate(template string, values map[string]string) string {
	for field, value := range values {
		template = strings.ReplaceAll(template, "${"+field+"}", value)
	}
	return template
}
