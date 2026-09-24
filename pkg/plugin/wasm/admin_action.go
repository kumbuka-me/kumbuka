package wasm

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// adminActionModule adapts one sandboxed administrator action to Kumbuka's plugin host.
type adminActionModule struct {
	// rendererModule embeds the shared guest invocation state.
	rendererModule
}

// Run invokes the guest action inside the authenticated administrator request context.
func (m adminActionModule) Run(ctx context.Context) error {
	execution := ctx
	if execution == nil {
		execution = context.Background()
	}
	_, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageAdminAction),
	})
	return err
}
