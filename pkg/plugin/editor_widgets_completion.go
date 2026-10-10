package plugin

import (
	"fmt"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// validateEditorWidgetCompletionReferences verifies resource controls reference editor-completion modules owned by the same plugin.
func validateEditorWidgetCompletionReferences(widgets []EditorWidgetContribution, manifest pluginpackage.Manifest) error {
	completionModules := make(map[string]bool)
	for _, module := range manifest.Modules {
		if ModuleType(module.Type) == ModuleTypeEditorCompletion {
			completionModules[module.ID] = true
		}
	}
	for _, widget := range widgets {
		for _, setting := range widget.Settings {
			if setting.Type == EditorWidgetSettingResource && !completionModules[setting.CompletionModuleID] {
				return fmt.Errorf("visual editor widget %q references unknown editor-completion module %q", widget.ID, setting.CompletionModuleID)
			}
			if setting.ChoiceSource != nil {
				if err := validateEditorWidgetChoiceSource(widget, *setting.ChoiceSource, manifest); err != nil {
					return err
				}
			}
			for _, field := range setting.Fields {
				if field.ChoiceSource != nil {
					if err := validateEditorWidgetChoiceSource(widget, *field.ChoiceSource, manifest); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// validateEditorWidgetChoiceSource validates attribute and module references for one dynamic option source.
func validateEditorWidgetChoiceSource(widget EditorWidgetContribution, source EditorWidgetChoiceSource, manifest pluginpackage.Manifest) error {
	if !validEditorWidgetChoiceSourceNames(source) {
		return fmt.Errorf("visual editor widget %q has an invalid choice source", widget.ID)
	}
	attributeFound := false
	for _, attribute := range widget.Attributes {
		if attribute.Name == source.SourceAttribute && editorWidgetScalarAttribute(attribute) {
			attributeFound = true
		}
	}
	if !attributeFound {
		return fmt.Errorf("visual editor widget %q choice source references unknown attribute %q", widget.ID, source.SourceAttribute)
	}
	if err := validateEditorWidgetChoiceSourceModule(widget, source, manifest, source.SettingModuleID, ModuleTypeSettings); err != nil {
		return err
	}
	return validateEditorWidgetChoiceSourceModule(widget, source, manifest, source.ResourceModuleID, ModuleTypeAdminResource)
}

// validateEditorWidgetChoiceSourceModule validates one declared settings or resource module reference.
func validateEditorWidgetChoiceSourceModule(widget EditorWidgetContribution, source EditorWidgetChoiceSource, manifest pluginpackage.Manifest, moduleID string, moduleType ModuleType) error {
	if moduleID == "" {
		return nil
	}
	for _, module := range manifest.Modules {
		if module.ID != moduleID || ModuleType(module.Type) != moduleType {
			continue
		}
		for _, field := range module.Fields {
			if editorWidgetChoiceSourceField(field, source) {
				return nil
			}
		}
	}
	return fmt.Errorf("visual editor widget %q choice source references invalid module %q", widget.ID, moduleID)
}

// editorWidgetChoiceSourceField reports whether a field supplies the requested list columns.
func editorWidgetChoiceSourceField(field pluginpackage.ConfigurationField, source EditorWidgetChoiceSource) bool {
	return field.ID == source.ListField && ConfigurationFieldType(field.Type) == ConfigurationFieldList && choiceColumnsExist(field.Columns, source)
}

// validEditorWidgetChoiceSourceNames reports whether source identifiers are usable.
func validEditorWidgetChoiceSourceNames(source EditorWidgetChoiceSource) bool {
	if source.SettingModuleID == "" && source.ResourceModuleID == "" {
		return false
	}
	for _, name := range []string{source.SourceAttribute, source.ListField, source.ValueColumn, source.LabelColumn} {
		if !validID.MatchString(name) {
			return false
		}
	}
	return source.DefaultColumn == "" || validID.MatchString(source.DefaultColumn)
}

// choiceColumnsExist reports whether all configured option columns are available.
func choiceColumnsExist(columns []pluginpackage.ConfigurationField, source EditorWidgetChoiceSource) bool {
	found := map[string]bool{}
	for _, column := range columns {
		found[column.ID] = true
	}
	return found[source.ValueColumn] && found[source.LabelColumn] && (source.DefaultColumn == "" || found[source.DefaultColumn])
}
