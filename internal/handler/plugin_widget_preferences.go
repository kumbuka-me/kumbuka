package handler

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// hiddenPluginWidgets applies submitted visibility only to widgets that were actually presented.
// Disabled widgets keep their previous preference and removed widgets are discarded.
func hiddenPluginWidgets(items []plugin.LoadedPlugin, current, presented, visible []string) []string {
	currentSet := stringSet(current)
	presentedSet := stringSet(presented)
	visibleSet := stringSet(visible)
	hidden := make([]string, 0, len(current))

	for _, item := range items {
		for _, module := range item.Manifest.Modules {
			if module.Type != "widget" {
				continue
			}
			key := plugin.WidgetKey(item.Manifest.ID, module.ID)
			switch {
			case !item.Enabled:
				if currentSet[key] {
					hidden = append(hidden, key)
				}
			case !presentedSet[key]:
				if currentSet[key] {
					hidden = append(hidden, key)
				}
			case !visibleSet[key]:
				hidden = append(hidden, key)
			}
		}
	}

	return hidden
}

// stringSet builds a membership set from string values.
func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			result[value] = true
		}
	}
	return result
}
