package wasm

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// contentChangeModule adapts one sandboxed committed-content hook to Kumbuka's plugin host.
type contentChangeModule struct {
	// rendererModule owns the shared WASM invocation adapter.
	rendererModule
}

// Changed invokes the guest with canonical Markdown from one committed page mutation.
func (m contentChangeModule) Changed(ctx plugin.Context, request plugin.ContentChangeRequest) error {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)

	_, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageContentChange),
		Features:   ctx.Features,
		ContentChange: &sdk.ContentChangeContext{
			Page:           request.Page,
			PreviousSource: request.PreviousSource,
			Source:         request.Source,
		},
	})
	return err
}
