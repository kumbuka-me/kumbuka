package plugin

import (
	"context"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

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
			if ModuleType(module.Type) != ModuleTypeEditorCompletion {
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

// manifestResourceModule finds an admin-resource declaration by ID.
func manifestResourceModule(manifest pluginpackage.Manifest, resourceID string) (pluginpackage.Module, bool) {
	for _, module := range manifest.Modules {
		if ModuleType(module.Type) == ModuleTypeAdminResource && module.ID == resourceID {
			return module, true
		}
	}
	return pluginpackage.Module{}, false
}

// directEditorCompletionFields converts fields supported by the inline creation form.
func directEditorCompletionFields(fields []pluginpackage.ConfigurationField) ([]EditorCompletionField, bool) {
	result := make([]EditorCompletionField, 0, len(fields))
	for _, field := range fields {
		switch ConfigurationFieldType(field.Type) {
		case ConfigurationFieldText,
			ConfigurationFieldTextarea,
			ConfigurationFieldURL,
			ConfigurationFieldBoolean,
			ConfigurationFieldSelect,
			ConfigurationFieldColor:
		default:
			return nil, false
		}
		result = append(result, EditorCompletionField{
			ID:       field.ID,
			Name:     field.Name,
			Type:     ConfigurationFieldType(field.Type),
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
		if item, ok := m.loaded[id]; ok && item.metadata.Enabled {
			plugins = append(plugins, cloneLoaded(item.metadata))
		}
	}
	m.mu.Unlock()
	var result []EditorCompletionItem
	for _, item := range plugins {
		for _, module := range item.Manifest.Modules {
			if ModuleType(module.Type) != ModuleTypeEditorCompletion {
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
			if ModuleType(module.Type) == ModuleTypeEditorInsert {
				result = append(result, editorInsertView(id, module))
			}
		}
	}
	return result
}
