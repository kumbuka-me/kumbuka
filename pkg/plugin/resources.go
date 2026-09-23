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
	resourceNamespace      = pluginSettingsNamespace
	maxResourceKeyBytes    = 128
	maxResourceRecordBytes = 60 << 10
)

// ParameterError reports invalid request-local plugin export input.
type ParameterError struct {
	// PluginID identifies the plugin associated with parameter error.
	PluginID string
	// ModuleID identifies the module associated with parameter error.
	ModuleID string
	// Key is the lookup key for parameter error.
	Key string
	// Message contains the message associated with parameter error.
	Message string
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
	// SecretFields reports which secret fields already contain a persisted value.
	SecretFields map[string]bool
}

// EditorCompletionField describes one resource field that can be created from editor completion UI.
type EditorCompletionField struct {
	// ID identifies the backing resource field.
	ID string `json:"id"`
	// Name is the human-readable form label.
	Name string `json:"name"`
	// Type selects the generic editor control used for the field.
	Type string `json:"type"`
	// Required reports whether the field must contain a value.
	Required bool `json:"required"`
	// Key reports whether this field identifies the resource record.
	Key bool `json:"key"`
	// MaxBytes is the configured UTF-8 byte limit, when one is declared.
	MaxBytes int `json:"max_bytes,omitempty"`
	// Options contains the allowed values for select fields.
	Options []string `json:"options,omitempty"`
	// Default contains the manifest-provided initial value.
	Default string `json:"default,omitempty"`
}

// EditorCompletionProvider describes one resource-backed completion source.
type EditorCompletionProvider struct {
	// PluginID identifies the plugin that owns the completion provider.
	PluginID string `json:"plugin_id"`
	// ModuleID identifies the editor-completion module.
	ModuleID string `json:"module_id"`
	// ResourceID identifies the backing admin-resource module.
	ResourceID string `json:"resource_id"`
	// ResourceName is the human-readable resource collection name.
	ResourceName string `json:"resource_name"`
	// Trigger opens this completion provider.
	Trigger string `json:"trigger"`
	// Replacement formats a newly created resource record into Markdown.
	Replacement string `json:"replacement"`
	// LabelField selects the resource field displayed as the completion label.
	LabelField string `json:"label_field"`
	// DetailField selects the optional resource field displayed below the label.
	DetailField string `json:"detail_field,omitempty"`
	// Fields describes controls available for direct record creation.
	Fields []EditorCompletionField `json:"fields"`
	// CanCreate reports whether direct creation is supported before request authorization is applied.
	CanCreate bool `json:"can_create"`
}

// EditorCompletionItem is one concrete resource-backed editor completion.
type EditorCompletionItem struct {
	// PluginID and ModuleID identify the owning completion contribution.
	PluginID string `json:"plugin_id"`
	// ModuleID identifies the module associated with editor completion item.
	ModuleID string `json:"module_id"`
	// Trigger opens completion when typed immediately before the query.
	Trigger string `json:"trigger"`
	// Label and Detail are displayed by editor completion UI.
	Label string `json:"label"`
	// Detail stores the detail value used by editor completion item.
	Detail string `json:"detail,omitempty"`
	// Replacement is inserted when the item is selected.
	Replacement string `json:"replacement"`
}

// EditorInsertContribution is one active declarative editor action.
type EditorInsertContribution struct {
	// PluginID and ModuleID identify the owning contribution.
	PluginID string `json:"plugin_id"`
	// ModuleID identifies the module associated with editor insert contribution.
	ModuleID string `json:"module_id"`
	// Name and Description are displayed by editor insertion UI.
	Name string `json:"name"`
	// Description describes editor insert contribution.
	Description string `json:"description,omitempty"`
	// Markdown and Suffix describe inserted, wrapped, or line-prefixed source.
	Markdown string `json:"markdown"`
	// Suffix stores the suffix value used by editor insert contribution.
	Suffix string `json:"suffix,omitempty"`
	// Placeholder supplies default selected text for wrap and prefix actions.
	Placeholder string `json:"placeholder,omitempty"`
	// Mode and Group select generic editor behavior and toolbar placement.
	Mode string `json:"mode"`
	// Group stores the group value used by editor insert contribution.
	Group string `json:"group"`
	// Icon is the optional host icon shown for the action.
	Icon string `json:"icon,omitempty"`
	// Inline reports whether plain insertion should avoid block line breaks.
	Inline bool `json:"inline"`
}

// ResourceRecords returns administrator-safe records for one declared admin resource.
func (m *Manager) ResourceRecords(ctx context.Context, pluginID, moduleID string) ([]ResourceRecord, error) {
	module, err := m.resourceModule(pluginID, moduleID)
	if err != nil {
		return nil, err
	}
	records, err := ReadResourceRecords(ctx, m.values, pluginID, module)
	if err != nil {
		return nil, err
	}
	for index := range records {
		records[index] = MaskResourceSecrets(records[index], module)
	}
	return records, nil
}

// ReadResourceRecords decodes one resource collection from trusted plugin storage.
func ReadResourceRecords(ctx context.Context, storage Storage, pluginID string, module pluginpackage.Module) ([]ResourceRecord, error) {
	if storage == nil {
		return nil, nil
	}
	stored, err := storage.ListPluginValues(ctx, pluginID, resourceNamespace, resourcePrefix(module.ID))
	if err != nil {
		return nil, err
	}

	records := make([]ResourceRecord, 0, len(stored))
	for _, data := range stored {
		record, err := decodeResourceRecord(pluginID, module, data)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
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

// ReadResourceRecord returns one stored resource record by its case-insensitive key.
func ReadResourceRecord(ctx context.Context, storage Storage, pluginID string, module pluginpackage.Module, key string) (ResourceRecord, bool, error) {
	if storage == nil || !validResourceKey(strings.TrimSpace(key)) {
		return ResourceRecord{}, false, nil
	}
	data, found, err := storage.ReadPluginValue(ctx, pluginID, resourceNamespace, resourceStorageKey(module.ID, key))
	if err != nil || !found {
		return ResourceRecord{}, found, err
	}
	record, err := decodeResourceRecord(pluginID, module, data)
	return record, err == nil, err
}

// decodeResourceRecord decodes one trusted stored resource record and verifies its key field.
func decodeResourceRecord(pluginID string, module pluginpackage.Module, data []byte) (ResourceRecord, error) {
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		return ResourceRecord{}, fmt.Errorf("decode plugin resource %s.%s: %w", pluginID, module.ID, err)
	}
	key := values[resourceKeyField(module).ID]
	if key == "" {
		return ResourceRecord{}, fmt.Errorf("plugin resource %s.%s contains a record without its key field", pluginID, module.ID)
	}
	return ResourceRecord{Key: key, Values: values}, nil
}

// cloneResourceValues returns an independent copy of one resource value map.
func cloneResourceValues(values map[string]string) map[string]string {
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

// MaskResourceSecrets removes encrypted secret payloads while preserving configured-state metadata.
func MaskResourceSecrets(record ResourceRecord, module pluginpackage.Module) ResourceRecord {
	values := cloneResourceValues(record.Values)
	secrets := make(map[string]bool)
	for _, field := range module.Fields {
		if field.Type != "secret" {
			continue
		}
		secrets[field.ID] = values[field.ID] != ""
		values[field.ID] = ""
	}
	record.Values = values
	record.SecretFields = secrets
	return record
}

// RevealResourceSecrets decrypts secret fields before a record is returned to its owning plugin.
func RevealResourceSecrets(record ResourceRecord, module pluginpackage.Module, codec SecretCodec) (ResourceRecord, error) {
	values := cloneResourceValues(record.Values)
	for _, field := range module.Fields {
		if field.Type != "secret" || values[field.ID] == "" {
			continue
		}
		if codec == nil || !codec.Configured() {
			return ResourceRecord{}, ErrSecretEncryptionUnavailable
		}
		plain, err := codec.Decrypt(values[field.ID])
		if err != nil {
			return ResourceRecord{}, fmt.Errorf("decrypt plugin resource %s: %w", field.ID, err)
		}
		values[field.ID] = plain
	}
	record.Values = values
	record.SecretFields = nil
	return record, nil
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

	originalKey = strings.TrimSpace(originalKey)
	previous, err := readPreviousResourceRecord(ctx, m.values, pluginID, module, originalKey)
	if err != nil {
		return err
	}

	validated, key, err := normalizeResourceRecord(module, values, previous.Values, originalKey == "")
	if err != nil {
		return err
	}
	if err := m.encryptConfigurationSecrets(module, validated, previous.Values); err != nil {
		return err
	}
	encoded, err := json.Marshal(validated)
	if err != nil {
		return err
	}
	if len(encoded) > maxResourceRecordBytes {
		return errors.New("plugin resource record is too large")
	}

	return m.persistResourceRecord(ctx, pluginID, module, originalKey, key, encoded)
}

// readPreviousResourceRecord loads the existing record for an edit.
func readPreviousResourceRecord(ctx context.Context, storage Storage, pluginID string, module pluginpackage.Module, key string) (ResourceRecord, error) {
	if key == "" {
		return ResourceRecord{}, nil
	}
	record, found, err := ReadResourceRecord(ctx, storage, pluginID, module, key)
	if err != nil {
		return ResourceRecord{}, err
	}
	if !found {
		return ResourceRecord{}, errors.New("plugin resource record no longer exists")
	}
	return record, nil
}

// persistResourceRecord writes a record in place or atomically renames its storage key.
func (m *Manager) persistResourceRecord(ctx context.Context, pluginID string, module pluginpackage.Module, originalKey, key string, encoded []byte) error {
	newStorageKey := resourceStorageKey(module.ID, key)
	oldStorageKey := resourceStorageKey(module.ID, originalKey)
	if originalKey == "" || oldStorageKey == newStorageKey {
		return m.values.WritePluginValue(ctx, pluginID, resourceNamespace, newStorageKey, encoded)
	}

	err := m.values.ReplacePluginValue(ctx, pluginID, resourceNamespace, oldStorageKey, newStorageKey, encoded)
	switch {
	case errors.Is(err, ErrPluginValueAlreadyExists):
		keyField := resourceKeyField(module)
		return configurationFieldError(keyField, keyField.Name+" is already in use.")
	case errors.Is(err, ErrPluginValueNotFound):
		return errors.New("plugin resource record no longer exists")
	default:
		return err
	}
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

// EditorCompletionProviders returns active resource-backed completion definitions.
func (m *Manager) EditorCompletionProviders() []EditorCompletionProvider {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []EditorCompletionProvider
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}

		for _, module := range item.metadata.Manifest.Modules {
			if module.Type != "editor-completion" {
				continue
			}

			resource, ok := manifestResourceModule(item.metadata.Manifest, module.Resource)
			if !ok {
				continue
			}

			fields, supported := directEditorCompletionFields(resource.Fields)
			result = append(result, EditorCompletionProvider{
				PluginID:     id,
				ModuleID:     module.ID,
				ResourceID:   resource.ID,
				ResourceName: resource.Name,
				Trigger:      module.Trigger,
				Replacement:  module.Replacement,
				LabelField:   module.LabelField,
				DetailField:  module.DetailField,
				Fields:       fields,
				CanCreate:    supported,
			})
		}
	}
	return result
}

// manifestResourceModule finds one admin-resource declaration by ID.
func manifestResourceModule(manifest pluginpackage.Manifest, resourceID string) (pluginpackage.Module, bool) {
	for _, module := range manifest.Modules {
		if module.Type == "admin-resource" && module.ID == resourceID {
			return module, true
		}
	}
	return pluginpackage.Module{}, false
}

// directEditorCompletionFields converts resource fields supported by the inline editor form.
func directEditorCompletionFields(fields []pluginpackage.ConfigurationField) ([]EditorCompletionField, bool) {
	result := make([]EditorCompletionField, 0, len(fields))
	for _, field := range fields {
		switch field.Type {
		case "text", "textarea", "url", "boolean", "select", "color":
		default:
			return nil, false
		}

		result = append(result, EditorCompletionField{
			ID:       field.ID,
			Name:     field.Name,
			Type:     field.Type,
			Required: field.Required,
			Key:      field.Key,
			MaxBytes: field.MaxBytes,
			Options:  append([]string(nil), field.Options...),
			Default:  field.Default,
		})
	}
	return result, len(result) > 0
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
func resourceKeyField(module pluginpackage.Module) pluginpackage.ConfigurationField {
	for _, field := range module.Fields {
		if field.Key {
			return field
		}
	}
	return pluginpackage.ConfigurationField{}
}

// normalizeResourceRecord validates and normalizes one record against its manifest schema.
func normalizeResourceRecord(module pluginpackage.Module, values, previous map[string]string, creating bool) (map[string]string, string, error) {
	if err := validateResourceFieldNames(module, values); err != nil {
		return nil, "", err
	}

	result := make(map[string]string, len(module.Fields))
	key := ""
	for _, field := range module.Fields {
		normalized, err := normalizeResourceField(field, values[field.ID], previous, creating)
		if err != nil {
			return nil, "", err
		}
		if field.Key {
			if !validResourceKey(normalized) {
				return nil, "", configurationFieldError(field, field.Name+" contains unsupported characters.")
			}
			key = normalized
		}
		result[field.ID] = normalized
	}
	return result, key, nil
}

// validateResourceFieldNames rejects values not declared by the resource module.
func validateResourceFieldNames(module pluginpackage.Module, values map[string]string) error {
	declared := make(map[string]struct{}, len(module.Fields))
	for _, field := range module.Fields {
		declared[field.ID] = struct{}{}
	}
	for field := range values {
		if _, ok := declared[field]; !ok {
			return fmt.Errorf("unknown resource field %q", field)
		}
	}
	return nil
}

// normalizeResourceField applies defaults and manifest validation to one field.
func normalizeResourceField(field pluginpackage.ConfigurationField, value string, previous map[string]string, creating bool) (string, error) {
	if creating && value == "" && field.Default != "" {
		value = field.Default
	}
	normalized, err := normalizeConfigurationValue(field, value)
	if err != nil {
		return "", err
	}
	if requiredConfigurationValueMissing(field, normalized, previous) {
		return "", configurationFieldError(field, field.Name+" is required.")
	}
	return normalized, nil
}

// validResourceKey reports whether key can be used in a macro and plugin storage key.
func validResourceKey(key string) bool {
	if key == "" || len(key) > maxResourceKeyBytes || !utf8.ValidString(key) {
		return false
	}

	return strings.TrimSpace(key) == key && !strings.ContainsAny(key, "\x00\r\n{}")
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
