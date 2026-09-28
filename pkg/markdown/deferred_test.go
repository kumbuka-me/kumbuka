package markdown

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type deferredTestMacro struct{ calls *atomic.Int32 }

func (deferredTestMacro) Name() string { return "remote" }
func (deferredTestMacro) SourceUsage() plugin.SourceUsage {
	return plugin.SourceUsage{ModuleID: "remote-files", Rules: []plugin.SourceUsageRule{{Macro: "remote"}}}
}
func (deferredTestMacro) Parse(source string) (plugin.Invocation, bool) {
	if source != `{{remote path="README.md"}}` {
		return nil, false
	}
	return plugin.Invocation(`{"path":"README.md"}`), true
}
func (m deferredTestMacro) Render(plugin.Context, plugin.Invocation) (string, error) {
	m.calls.Add(1)
	return `<div class="remote-result">loaded<script>bad()</script></div>`, nil
}

func TestDeferredMacroSkipsInitialRenderAndLoadsExactFragment(t *testing.T) {
	var calls atomic.Int32
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(
		plugin.Descriptor{ID: "io.example.remote", Name: "Remote"},
		plugin.Contributions{Macros: []plugin.Macro{deferredTestMacro{calls: &calls}}},
	))
	renderer := NewWithRegistry(registry)
	source := `{{remote path="README.md"}}`
	functions := Functions{
		Context:         context.Background(),
		Locale:          "de-CH",
		DeferredVersion: "1234",
		DeferMacro: func(pluginID, moduleID string) bool {
			return pluginID == "io.example.remote" && moduleID == "remote-files"
		},
	}

	page, err := renderer.RenderPageResolvedWithFunctions(source, Slug, DefaultOptions(), functions)
	require.NoError(t, err)
	assert.Zero(t, calls.Load())
	assert.Contains(t, page.HTML, `data-kumbuka-deferred-plugin="io.example.remote"`)
	assert.Contains(t, page.HTML, `data-kumbuka-deferred-module="remote-files"`)
	assert.Contains(t, page.HTML, `data-kumbuka-deferred-version="1234"`)
	assert.Contains(t, page.HTML, `data-kumbuka-deferred-locale="de-CH"`)

	fragment, err := renderer.RenderDeferredMacro(
		source, 0, "io.example.remote", "remote-files", Slug, DefaultOptions(), functions,
	)
	require.NoError(t, err)
	assert.Equal(t, int32(1), calls.Load())
	assert.Contains(t, fragment, `<div class="remote-result">loaded</div>`)
	assert.NotContains(t, fragment, "script")

	_, err = renderer.RenderDeferredMacro(
		source, 0, "io.example.other", "remote-files", Slug, DefaultOptions(), functions,
	)
	assert.ErrorIs(t, err, ErrDeferredMacroNotFound)
}
