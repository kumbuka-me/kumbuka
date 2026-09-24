package wasm

import (
	"context"
	"errors"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// widgetModule adapts one sandboxed widget declaration to Kumbuka's widget host.
type widgetModule struct {
	// rendererModule embeds renderer module behavior in widget module.
	rendererModule
}

// Render invokes the guest widget and assembles untrusted HTML plus safe host actions.
func (m widgetModule) Render(ctx plugin.Context, request plugin.WidgetRequest) (plugin.WidgetResult, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)

	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageWidget),
		Features:   ctx.Features,
		Widget:     &sdk.WidgetContext{Surface: request.Surface, Page: request.Page},
	})
	if err != nil {
		return plugin.WidgetResult{}, err
	}

	var output strings.Builder
	for _, part := range result.Parts {
		if part.Markdown != nil {
			return plugin.WidgetResult{}, errors.New("widget output cannot contain Markdown fragments")
		}
		if len(part.Text) > m.instance.runtime.limits.WireBytes-output.Len() {
			return plugin.WidgetResult{}, errors.New("rendered widget output exceeds size limit")
		}
		output.WriteString(part.Text)
	}

	return plugin.WidgetResult{HTML: output.String(), Actions: result.Actions}, nil
}

// Command invokes one host-mediated widget command in the guest.
func (m widgetModule) Command(ctx plugin.Context, request plugin.WidgetCommandRequest) (sdk.WidgetCommandResult, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)

	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageWidgetCommand),
		Features:   ctx.Features,
		WidgetCommand: &sdk.WidgetCommandContext{
			Surface: request.Surface,
			Page:    request.Page,
			Action:  request.Action,
		},
	})
	if err != nil {
		return sdk.WidgetCommandResult{}, err
	}
	if result.WidgetCommand == nil {
		return sdk.WidgetCommandResult{}, errors.New("widget command returned no result")
	}
	return *result.WidgetCommand, nil
}
