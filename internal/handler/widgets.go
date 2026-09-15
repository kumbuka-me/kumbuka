package handler

import (
	"html/template"

	md "github.com/kumbuka-me/kumbuka/internal/markdown"
)

func widgetViews(widgets []md.RenderedWidget) []pluginWidgetView {
	result := make([]pluginWidgetView, 0, len(widgets))
	for _, widget := range widgets {
		result = append(result, pluginWidgetView{
			PluginID: widget.PluginID,
			ModuleID: widget.ModuleID,
			Width:    widget.Width,
			HTML:     template.HTML(widget.HTML),
			Actions:  widget.Actions,
		})
	}
	return result
}
