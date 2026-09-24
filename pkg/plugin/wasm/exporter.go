package wasm

import (
	"context"
	"errors"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// exporterModule adapts one sandboxed exporter declaration to Kumbuka's export host.
type exporterModule struct {
	// rendererModule embeds the shared guest invocation state.
	rendererModule
}

// Export invokes the guest exporter and validates its bounded file metadata.
func (m exporterModule) Export(ctx plugin.Context, request plugin.ExportRequest) (sdk.ExportFile, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)

	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageExport),
		Features:   ctx.Features,
		Export:     &sdk.ExportContext{Page: request.Page, Source: request.Source},
	})
	if err != nil {
		return sdk.ExportFile{}, err
	}
	if result.File == nil {
		return sdk.ExportFile{}, errors.New("invalid plugin export file")
	}
	return *result.File, nil
}
