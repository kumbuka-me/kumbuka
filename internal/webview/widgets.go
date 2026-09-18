package webview

import (
	"html/template"
	"net/url"
	"strings"

	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// Widgets converts rendered plugin widgets into template view models.
func Widgets(widgets []md.RenderedWidget, surface, pageSlug, next string) []Widget {
	result := make([]Widget, 0, len(widgets))
	for _, widget := range widgets {
		result = append(result, Widget{
			PluginID: widget.PluginID,
			ModuleID: widget.ModuleID,
			Width:    widget.Width,
			HTML:     template.HTML(widget.HTML),
			Actions:  widgetActions(widget.PluginID, widget.ModuleID, surface, pageSlug, next, widget.Actions),
		})
	}
	return result
}

// widgetActions converts plugin actions into host-owned links, dialogs, or command forms.
func widgetActions(pluginID, moduleID, surface, pageSlug, next string, actions []sdk.WidgetAction) []WidgetAction {
	result := make([]WidgetAction, 0, len(actions))
	for _, action := range actions {
		target := action.URL
		if action.Kind == "command" {
			target = "/plugins/actions/" + url.PathEscape(pluginID) + "/" + url.PathEscape(moduleID) + "/" + url.PathEscape(action.ID)
		}
		result = append(result, WidgetAction{
			ID: action.ID, Kind: action.Kind, Label: action.Label, URL: target, Icon: action.Icon, Confirm: action.Confirm,
			Surface: surface, PageSlug: pageSlug, Next: next,
		})
	}
	return result
}

// pluginWidgetPreferences returns enabled widgets as user-facing visibility controls.
func pluginWidgetPreferences(items []plugin.LoadedPlugin, hidden []string) []WidgetPreference {
	hiddenSet := stringSet(hidden)
	preferences := make([]WidgetPreference, 0)

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
			preferences = append(preferences, WidgetPreference{
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
