package site

import (
	"context"
	"testing"

	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/stretchr/testify/require"
)

// testMarkdownRenderer returns a core renderer without starting the plugin runtime.
func testMarkdownRenderer(t testing.TB) *md.Renderer {
	t.Helper()
	return md.NewWithRegistry(nil)
}

// testPluginMarkdownRenderer returns a renderer with only the requested bundled plugins.
func testPluginMarkdownRenderer(t testing.TB, names ...string) (*md.Renderer, *plugin.Manager) {
	t.Helper()
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("pages:read", "pages:content", "browser:render"))
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })

	archives := make([][]byte, 0, len(names))
	for _, name := range names {
		archive, err := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		require.NoError(t, err)
		archives = append(archives, archive)
	}
	require.NoError(t, manager.Bootstrap(ctx, archives))
	return md.NewWithManager(registry, manager), manager
}
