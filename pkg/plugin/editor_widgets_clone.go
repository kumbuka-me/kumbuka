package plugin

import "github.com/kumbuka-me/kumbuka/pkg/utils"

// cloneEditorWidget copies mutable contract fields before they leave the manager.
func cloneEditorWidget(widget EditorWidgetContribution) EditorWidgetContribution {
	clone := widget
	clone.Attributes = append([]EditorWidgetAttribute(nil), widget.Attributes...)
	for index := range clone.Attributes {
		clone.Attributes[index].Values = append([]string(nil), widget.Attributes[index].Values...)
		clone.Attributes[index].Aliases = cloneStringMap(widget.Attributes[index].Aliases)
	}
	clone.Settings = append([]EditorWidgetSetting(nil), widget.Settings...)
	for index := range clone.Settings {
		clone.Settings[index].Attributes = append([]string(nil), widget.Settings[index].Attributes...)
		clone.Settings[index].Columns = append([]EditorWidgetSettingColumn(nil), widget.Settings[index].Columns...)
		clone.Settings[index].Suggestions = append([]string(nil), widget.Settings[index].Suggestions...)
		clone.Settings[index].Fields = append([]EditorWidgetTreeField(nil), widget.Settings[index].Fields...)
		clone.Settings[index].ChoiceSource = cloneEditorWidgetChoiceSource(widget.Settings[index].ChoiceSource)
		clone.Settings[index].Choices = cloneEditorWidgetChoices(widget.Settings[index].Choices)
		for fieldIndex := range clone.Settings[index].Fields {
			clone.Settings[index].Fields[fieldIndex].Suggestions = append([]string(nil), widget.Settings[index].Fields[fieldIndex].Suggestions...)
			clone.Settings[index].Fields[fieldIndex].ChoiceSource = cloneEditorWidgetChoiceSource(widget.Settings[index].Fields[fieldIndex].ChoiceSource)
			clone.Settings[index].Fields[fieldIndex].Choices = cloneEditorWidgetChoices(widget.Settings[index].Fields[fieldIndex].Choices)
		}
	}
	clone.Constraints = append([]EditorWidgetConstraint(nil), widget.Constraints...)
	for index := range clone.Constraints {
		clone.Constraints[index].Attributes = append([]string(nil), widget.Constraints[index].Attributes...)
	}
	if widget.Preview.Badge != nil {
		badge := *widget.Preview.Badge
		badge.DefaultColors = append([]string(nil), widget.Preview.Badge.DefaultColors...)
		badge.ToneClasses = cloneStringMap(widget.Preview.Badge.ToneClasses)
		clone.Preview.Badge = &badge
	}
	if widget.Preview.Reference != nil {
		clone.Preview.Reference = utils.ToPtr(*widget.Preview.Reference)
	}
	if widget.Preview.Card != nil {
		card := *widget.Preview.Card
		card.MetadataAttributes = append([]string(nil), widget.Preview.Card.MetadataAttributes...)
		if widget.Preview.Card.LineAnnotations != nil {
			card.LineAnnotations = utils.ToPtr(*widget.Preview.Card.LineAnnotations)
		}
		clone.Preview.Card = &card
	}
	if widget.Preview.Callout != nil {
		clone.Preview.Callout = utils.ToPtr(*widget.Preview.Callout)
	}
	if widget.Preview.Details != nil {
		clone.Preview.Details = utils.ToPtr(*widget.Preview.Details)
	}
	if widget.Preview.Tabs != nil {
		clone.Preview.Tabs = utils.ToPtr(*widget.Preview.Tabs)
	}
	return clone
}

// cloneEditorWidgetChoiceSource copies an optional choice-source declaration.
func cloneEditorWidgetChoiceSource(source *EditorWidgetChoiceSource) *EditorWidgetChoiceSource {
	if source == nil {
		return nil
	}
	clone := *source
	return &clone
}

// cloneEditorWidgetChoices copies option lists while preserving nil.
func cloneEditorWidgetChoices(source map[string][]EditorWidgetChoice) map[string][]EditorWidgetChoice {
	if source == nil {
		return nil
	}
	clone := make(map[string][]EditorWidgetChoice, len(source))
	for key, choices := range source {
		clone[key] = append([]EditorWidgetChoice(nil), choices...)
	}
	return clone
}

// cloneStringMap copies a string map while preserving nil.
func cloneStringMap(source map[string]string) map[string]string {
	if source == nil {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}
