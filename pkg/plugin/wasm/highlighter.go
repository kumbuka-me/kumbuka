package wasm

import (
	"context"
	"errors"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
)

// codeHighlighterModule adapts one sandboxed code-highlighter declaration.
type codeHighlighterModule struct {
	// rendererModule embeds renderer module behavior in code highlighter module.
	rendererModule
}

// Highlight invokes the guest for one fenced code block.
func (m codeHighlighterModule) Highlight(ctx plugin.Context, language, source string) (plugin.CodeHighlightResult, error) {
	execution := ctx.Context
	if execution == nil {
		execution = context.Background()
	}
	execution = context.WithValue(execution, capabilitiesKey{}, ctx.Capabilities)

	result, err := m.instance.invoke(execution, sdk.RenderRequest{
		APIVersion: sdk.Version,
		Module:     m.module.ID,
		Stage:      string(plugin.RenderStageHighlight),
		Source:     source,
		Language:   language,
		Features:   ctx.Features,
	})
	if err != nil {
		return plugin.CodeHighlightResult{}, err
	}
	if !result.Matched {
		return plugin.CodeHighlightResult{}, nil
	}

	var output strings.Builder
	for _, part := range result.Parts {
		if err := execution.Err(); err != nil {
			return plugin.CodeHighlightResult{}, err
		}
		if len(part.Text) > m.instance.runtime.limits.WireBytes-output.Len() {
			return plugin.CodeHighlightResult{}, errors.New("highlighted plugin output exceeds size limit")
		}
		output.WriteString(part.Text)
	}

	return plugin.CodeHighlightResult{HTML: output.String(), Matched: true}, nil
}
