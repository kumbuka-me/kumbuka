package markdown

import (
	"context"
	"errors"

	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/sdk"
)

// RenderedWidget contains one sanitized plugin widget ready for a host template.
type RenderedWidget struct {
	PluginID string
	ModuleID string
	Width    string
	HTML     string
	Actions  []sdk.WidgetAction
}

// RenderWidgets invokes active widgets for one surface and sanitizes every guest fragment.
func (r *Renderer) RenderWidgets(
	ctx context.Context,
	surface string,
	page *sdk.Page,
	features map[string]bool,
	capabilities map[string]plugin.Capability,
	hiddenWidgets []string,
) ([]RenderedWidget, error) {
	if !sdk.ValidWidgetSurface(surface) {
		return nil, errors.New("invalid widget surface")
	}
	if r.registry == nil {
		return nil, nil
	}

	plan, release := r.registry.AcquireRenderPlan()
	defer release()

	widgetContext := plugin.Context{
		Context:      ctx,
		Capabilities: capabilities,
		Features:     plan.RenderFeatures(features),
	}
	request := plugin.WidgetRequest{Surface: surface, Page: page}
	result := make([]RenderedWidget, 0, len(plan.Widgets))
	hidden := make(map[string]bool, len(hiddenWidgets))
	for _, key := range hiddenWidgets {
		hidden[key] = true
	}

	for _, binding := range plan.Widgets {
		if binding.Surface != surface || hidden[plugin.WidgetKey(binding.PluginID, binding.ModuleID)] {
			continue
		}
		rendered, err := binding.Module.Render(widgetContext, request)
		if err != nil {
			return nil, err
		}
		html := r.sanitizer.Sanitize(rendered.HTML)
		if html == "" && len(rendered.Actions) == 0 {
			continue
		}
		result = append(result, RenderedWidget{
			PluginID: binding.PluginID,
			ModuleID: binding.ModuleID,
			Width:    binding.Width,
			HTML:     html,
			Actions:  append([]sdk.WidgetAction(nil), rendered.Actions...),
		})
	}

	return result, nil
}
