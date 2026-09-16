package handler

import (
	"html/template"
	"net/url"

	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/sdk"
)

// widgetViews converts rendered plugin widgets into template view models.
func widgetViews(widgets []md.RenderedWidget, surface, pageSlug, next string) []pluginWidgetView {
	result := make([]pluginWidgetView, 0, len(widgets))
	for _, widget := range widgets {
		result = append(result, pluginWidgetView{
			PluginID: widget.PluginID,
			ModuleID: widget.ModuleID,
			Width:    widget.Width,
			HTML:     template.HTML(widget.HTML),
			Actions:  widgetActionViews(widget.PluginID, widget.ModuleID, surface, pageSlug, next, widget.Actions),
		})
	}
	return result
}

// widgetActionViews converts plugin actions into host-owned links, dialogs, or command forms.
func widgetActionViews(pluginID, moduleID, surface, pageSlug, next string, actions []sdk.WidgetAction) []pluginWidgetActionView {
	result := make([]pluginWidgetActionView, 0, len(actions))
	for _, action := range actions {
		target := action.URL
		if action.Kind == "command" {
			target = "/plugins/actions/" + url.PathEscape(pluginID) + "/" + url.PathEscape(moduleID) + "/" + url.PathEscape(action.ID)
		}
		result = append(result, pluginWidgetActionView{
			ID: action.ID, Kind: action.Kind, Label: action.Label, URL: target, Icon: action.Icon, Confirm: action.Confirm,
			Surface: surface, PageSlug: pageSlug, Next: next,
		})
	}
	return result
}
