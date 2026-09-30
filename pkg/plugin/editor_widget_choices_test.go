package plugin

import (
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeEditorWidgetChoicesUsesConfiguredDefault(t *testing.T) {
	source := EditorWidgetChoiceSource{ValueColumn: "id", LabelColumn: "label", DefaultColumn: "default"}
	choices, err := decodeEditorWidgetChoices(`[{"id":"open","label":"Open","default":"false"},{"id":"review","label":"Review","default":"true"}]`, source)
	require.NoError(t, err)
	require.Len(t, choices, 2)
	assert.False(t, choices[0].Default)
	assert.True(t, choices[1].Default)

	_, err = decodeEditorWidgetChoices(`[{"id":"open","label":"Open","default":"true"},{"id":"review","label":"Review","default":"true"}]`, source)
	require.ErrorContains(t, err, "only one")
}

func TestEditorWidgetChoiceSourceReferencesDeclaredLists(t *testing.T) {
	source := &EditorWidgetChoiceSource{
		SettingModuleID: "workflow", ResourceModuleID: "workflows", SourceAttribute: "workflow",
		ListField: "states", ValueColumn: "id", LabelColumn: "label", DefaultColumn: "default",
	}
	widget := EditorWidgetContribution{
		ID: "task",
		Attributes: []EditorWidgetAttribute{
			{Name: "workflow", Type: EditorWidgetAttributeIdentifier},
			{Name: "initial", Type: EditorWidgetAttributeString},
		},
		Settings: []EditorWidgetSetting{{Type: EditorWidgetSettingSelect, Label: "Initial state", Attribute: "initial", ChoiceSource: source}},
	}
	columns := []pluginpackage.ConfigurationField{
		{ID: "id", Type: "text"}, {ID: "label", Type: "text"}, {ID: "default", Type: "select"},
	}
	manifest := pluginpackage.Manifest{Modules: []pluginpackage.Module{
		{Type: "settings", ID: "workflow", Fields: []pluginpackage.ConfigurationField{{ID: "states", Type: "list", Columns: columns}}},
		{Type: "admin-resource", ID: "workflows", Fields: []pluginpackage.ConfigurationField{{ID: "states", Type: "list", Columns: columns}}},
	}}
	require.NoError(t, validateEditorWidgetCompletionReferences([]EditorWidgetContribution{widget}, manifest))
}

func TestEditorWidgetChoiceSourceRejectsWrongModuleType(t *testing.T) {
	for _, moduleType := range []string{"settings", "admin-resource"} {
		t.Run(moduleType, func(t *testing.T) {
			source := EditorWidgetChoiceSource{SourceAttribute: "workflow", ListField: "states", ValueColumn: "id", LabelColumn: "label"}
			if moduleType == "settings" {
				source.ResourceModuleID = "workflow"
			} else {
				source.SettingModuleID = "workflow"
			}
			widget := EditorWidgetContribution{ID: "task", Attributes: []EditorWidgetAttribute{{Name: "workflow", Type: EditorWidgetAttributeIdentifier}}}
			manifest := pluginpackage.Manifest{Modules: []pluginpackage.Module{{ID: "workflow", Type: moduleType, Fields: []pluginpackage.ConfigurationField{{ID: "states", Type: "list", Columns: []pluginpackage.ConfigurationField{{ID: "id", Type: "text"}, {ID: "label", Type: "text"}}}}}}}
			require.ErrorContains(t, validateEditorWidgetChoiceSource(widget, source, manifest), "invalid module")
		})
	}
}
