package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// EditorWidgetsWithChoices resolves explicitly declared list-backed select options for the editor actor.
func (m *Manager) EditorWidgetsWithChoices(ctx context.Context) ([]EditorWidgetContribution, []EditorWidgetProblem, error) {
	widgets, problems := m.EditorWidgets()
	result := make([]EditorWidgetContribution, 0, len(widgets))
	for _, widget := range widgets {
		if err := m.resolveEditorWidgetChoices(ctx, &widget); err != nil {
			problems = append(problems, editorWidgetProblem(widget.PluginID, err.Error()))
			continue
		}
		result = append(result, widget)
	}
	return result, problems, nil
}

func (m *Manager) resolveEditorWidgetChoices(ctx context.Context, widget *EditorWidgetContribution) error {
	cache := make(map[EditorWidgetChoiceSource]map[string][]EditorWidgetChoice)
	resolve := func(source *EditorWidgetChoiceSource) (map[string][]EditorWidgetChoice, error) {
		if source == nil {
			return nil, nil
		}
		if choices, ok := cache[*source]; ok {
			return cloneEditorWidgetChoices(choices), nil
		}
		choices, err := m.loadEditorWidgetChoices(ctx, widget.PluginID, *source)
		if err != nil {
			return nil, err
		}
		cache[*source] = choices
		return cloneEditorWidgetChoices(choices), nil
	}

	for settingIndex := range widget.Settings {
		setting := &widget.Settings[settingIndex]
		choices, err := resolve(setting.ChoiceSource)
		if err != nil {
			return err
		}
		setting.Choices = choices
		for fieldIndex := range setting.Fields {
			field := &setting.Fields[fieldIndex]
			choices, err = resolve(field.ChoiceSource)
			if err != nil {
				return err
			}
			field.Choices = choices
		}
	}
	return nil
}

func (m *Manager) loadEditorWidgetChoices(ctx context.Context, pluginID string, source EditorWidgetChoiceSource) (map[string][]EditorWidgetChoice, error) {
	result := make(map[string][]EditorWidgetChoice)
	if source.SettingModuleID != "" {
		groups, err := m.SettingGroups(ctx, pluginID)
		if err != nil {
			return nil, err
		}
		for _, group := range groups {
			if group.Module.ID != source.SettingModuleID {
				continue
			}
			choices, err := decodeEditorWidgetChoices(group.Values[source.ListField], source)
			if err != nil {
				return nil, fmt.Errorf("visual editor choices from %s are invalid: %w", source.SettingModuleID, err)
			}
			result[""] = choices
		}
	}
	if source.ResourceModuleID != "" {
		records, err := m.ResourceRecords(ctx, pluginID, source.ResourceModuleID)
		if err != nil {
			return nil, err
		}
		for _, record := range records {
			choices, err := decodeEditorWidgetChoices(record.Values[source.ListField], source)
			if err != nil {
				return nil, fmt.Errorf("visual editor choices from %s %q are invalid: %w", source.ResourceModuleID, record.Key, err)
			}
			result[record.Key] = choices
		}
	}
	return result, nil
}

func decodeEditorWidgetChoices(value string, source EditorWidgetChoiceSource) ([]EditorWidgetChoice, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var rows []map[string]string
	if err := json.Unmarshal([]byte(value), &rows); err != nil {
		return nil, err
	}
	choices := make([]EditorWidgetChoice, 0, len(rows))
	seen := make(map[string]bool, len(rows))
	defaultSeen := false
	for _, row := range rows {
		choice := EditorWidgetChoice{Value: strings.TrimSpace(row[source.ValueColumn]), Label: strings.TrimSpace(row[source.LabelColumn])}
		if choice.Value == "" || choice.Label == "" || seen[choice.Value] {
			return nil, fmt.Errorf("choice values and labels must be non-empty and unique")
		}
		seen[choice.Value] = true
		if source.DefaultColumn != "" {
			switch strings.TrimSpace(row[source.DefaultColumn]) {
			case "", "false":
			case "true":
				if defaultSeen {
					return nil, fmt.Errorf("only one choice may be the default")
				}
				choice.Default = true
				defaultSeen = true
			default:
				return nil, fmt.Errorf("default choice marker must be true or false")
			}
		}
		choices = append(choices, choice)
	}
	if len(choices) > 0 && !defaultSeen {
		choices[0].Default = true
	}
	return choices, nil
}
