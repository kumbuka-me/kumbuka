package wasm

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// macroModule adapts one WASM macro declaration to Kumbuka's macro contract.
type macroModule struct {
	// rendererModule embeds renderer module behavior in macro module.
	rendererModule
}

// Name returns the registered contribution name.
func (m macroModule) Name() string { return m.module.Name }

// Available reports whether the contribution is available in the current context.
func (m macroModule) Available(ctx plugin.Context) bool {
	if enabled, configured := ctx.Features[m.instance.manifest.ID]; configured && !enabled {
		return false
	}
	return m.module.Capability == "" || ctx.Capabilities[m.module.Capability] != nil
}

// Parse recognizes one macro invocation through the guest parse stage.
func (m macroModule) Parse(string) (plugin.Invocation, bool) { return nil, false }

// ParseContext parses context.
func (m macroModule) ParseContext(ctx plugin.Context, line string) (plugin.Invocation, bool, error) {
	if !strings.HasPrefix(strings.TrimSpace(line), "{{"+m.Name()) {
		return nil, false, nil
	}
	result, err := m.invoke(ctx, "parse", line, nil)
	return result.Invocation, result.Matched, err
}

// Render expands one parsed macro invocation through the guest macro stage.
func (m macroModule) Render(ctx plugin.Context, invocation plugin.Invocation) (string, error) {
	result, err := m.invoke(ctx, "macro", "", invocation)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	for _, part := range result.Parts {
		output.WriteString(part.Text)
	}
	return output.String(), nil
}

// invoke sends one macro stage request through the owning WASM instance.
func (m macroModule) invoke(ctx plugin.Context, stage, source string, invocation json.RawMessage) (sdk.RenderResult, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)
	return m.instance.invoke(execution, sdk.RenderRequest{APIVersion: sdk.Version, Module: m.module.ID, Stage: stage, Source: source, Invocation: invocation})
}
