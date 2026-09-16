package handler

import (
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// pluginWidgetPreferenceView contains template data for plugin widget preference view.
type pluginWidgetPreferenceView struct {
	// Key is the lookup key for plugin widget preference view.
	Key string
	// Label is the display label for plugin widget preference view.
	Label string
	// Surface stores the surface value used by plugin widget preference view.
	Surface string
	// Description describes plugin widget preference view.
	Description string
	// Visible reports whether visible applies to plugin widget preference view.
	Visible bool
}

// pluginWidgetPreferences returns enabled widgets as generic user-facing visibility controls.
func pluginWidgetPreferences(items []plugin.LoadedPlugin, hidden []string) []pluginWidgetPreferenceView {
	hiddenSet := stringSet(hidden)
	preferences := make([]pluginWidgetPreferenceView, 0)

	for _, item := range items {
		if !item.Enabled {
			continue
		}
		for _, module := range item.Manifest.Modules {
			if module.Type != "widget" {
				continue
			}

			label := item.Manifest.Name
			if name := strings.TrimSpace(module.Name); name != "" {
				label += " · " + name
			}
			description := strings.TrimSpace(module.Description)
			if description == "" {
				description = strings.TrimSpace(item.Manifest.Description)
			}
			key := plugin.WidgetKey(item.Manifest.ID, module.ID)
			preferences = append(preferences, pluginWidgetPreferenceView{
				Key:         key,
				Label:       label,
				Surface:     widgetSurfaceLabel(module.Surface),
				Description: description,
				Visible:     !hiddenSet[key],
			})
		}
	}

	return preferences
}

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

// widgetSurfaceLabel returns the human-readable label for a widget surface.
func widgetSurfaceLabel(surface string) string {
	switch surface {
	case "home":
		return "Home"
	case "sidebar":
		return "Sidebar"
	case "page.details":
		return "Page details"
	default:
		return surface
	}
}
