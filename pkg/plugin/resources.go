package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

const (
	resourceNamespace      = "settings"
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
			return ResourceRecord{}, errors.New("plugin secret encryption is unavailable")
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

	var previous ResourceRecord
	if strings.TrimSpace(originalKey) != "" {
		var found bool
		previous, found, err = ReadResourceRecord(ctx, m.values, pluginID, module, originalKey)
		if err != nil {
			return err
		}
		if !found {
			return errors.New("plugin resource record no longer exists")
		}
	}

	validated, key, err := normalizeResourceRecord(module, values, previous.Values, strings.TrimSpace(originalKey) == "")
	if err != nil {
		return err
	}
	if err := m.encryptResourceSecrets(module, validated, previous.Values); err != nil {
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
			if _, found, readErr := m.values.ReadPluginValue(ctx, pluginID, resourceNamespace, newStorageKey); readErr != nil {
				return readErr
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

// encryptResourceSecrets replaces submitted plaintext secrets with encrypted persisted values.
func (m *Manager) encryptResourceSecrets(module pluginpackage.Module, values, previous map[string]string) error {
	for _, field := range module.Fields {
		if field.Type != "secret" {
			continue
		}
		plain := values[field.ID]
		if plain == "" && previous[field.ID] != "" {
			values[field.ID] = previous[field.ID]
			continue
		}
		if plain == "" {
			if field.Required {
				return fmt.Errorf("%s is required", field.Name)
			}
			continue
		}
		if m.secrets == nil || !m.secrets.Configured() {
			return errors.New("configure the application encryption key before saving plugin secrets")
		}
		encrypted, err := m.secrets.Encrypt(plain)
		if err != nil {
			return errors.New("could not encrypt plugin secret")
		}
		values[field.ID] = encrypted
	}
	return nil
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

// normalizeResourceRecord validates and normalizes one record against its manifest schema.
func normalizeResourceRecord(module pluginpackage.Module, values, previous map[string]string, creating bool) (map[string]string, string, error) {
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
		if creating && value == "" && field.Default != "" {
			value = field.Default
		}
		normalized, err := normalizeResourceValue(field, value)
		if err != nil {
			return nil, "", err
		}
		if field.Required && normalized == "" && !(field.Type == "secret" && previous[field.ID] != "") {
			return nil, "", fmt.Errorf("%s is required", field.Name)
		}
		if field.Key {
			if !validResourceKey(normalized) {
				return nil, "", fmt.Errorf("%s contains unsupported characters", field.Name)
			}
			key = normalized
		}
		result[field.ID] = normalized
	}
	return result, key, nil
}

// normalizeResourceValue validates one typed resource field value.
func normalizeResourceValue(field pluginpackage.ResourceField, value string) (string, error) {
	if field.Type == "text" || field.Type == "url" || field.Type == "select" || field.Type == "boolean" || field.Key {
		value = strings.TrimSpace(value)
	}
	if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("%s must contain valid UTF-8 text", field.Name)
	}
	if field.Type == "secret" && strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return "", fmt.Errorf("%s must not contain control characters", field.Name)
	}

	limit := field.MaxBytes
	if limit == 0 {
		switch {
		case field.Key:
			limit = maxResourceKeyBytes
		case field.Type == "textarea":
			limit = 48 << 10
		default:
			limit = 4096
		}
	}
	if len(value) > limit {
		return "", fmt.Errorf("%s is too long", field.Name)
	}

	switch field.Type {
	case "text", "textarea", "secret":
		return value, nil
	case "boolean":
		if value != "true" && value != "false" {
			return "", fmt.Errorf("%s must be true or false", field.Name)
		}
		return value, nil
	case "select":
		if value == "" && !field.Required {
			return "", nil
		}
		for _, option := range field.Options {
			if value == option {
				return value, nil
			}
		}
		return "", fmt.Errorf("%s has an unsupported value", field.Name)
	case "url":
		if value == "" {
			return value, nil
		}
		parsed, err := url.ParseRequestURI(value)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
			return "", fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", field.Name)
		}
		return value, nil
	default:
		return "", fmt.Errorf("%s uses an unsupported field type", field.Name)
	}
}

// cloneResourceValues copies a resource values map before masking or decryption.
func cloneResourceValues(values map[string]string) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
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
