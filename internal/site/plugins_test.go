package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticBuildUsesLiveRendererRegistry(t *testing.T) {
	ctx := context.Background()
	renderer, manager := testPluginMarkdownRenderer(t, "callouts")
	root := t.TempDir()
	source := filepath.Join(root, "docs")
	output := filepath.Join(root, "site")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "index.md"), []byte("# Home\n\n!!! note\nDynamic plugin\n"), 0644))
	assets := fstest.MapFS{}
	for _, name := range staticBrowserAssets {
		assets[name] = &fstest.MapFile{Data: []byte("asset")}
	}
	config := defaultConfig()
	config.SourceDir = source
	config.OutputDir = output
	config.SiteURL = "https://example.com/"
	require.NoError(t, BuildWithRenderer(ctx, assets, config, renderer))
	html, err := os.ReadFile(filepath.Join(output, "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(html), `class="callout note"`)
	require.NoError(t, manager.Disable(ctx, "me.kumbuka.callouts"))
	require.NoError(t, BuildWithRenderer(ctx, assets, config, renderer))
	html, err = os.ReadFile(filepath.Join(output, "index.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(html), `class="callout note"`)
	require.NoError(t, manager.Enable(ctx, "me.kumbuka.callouts"))
}

func TestStaticBrowserPackagesFollowLiveRegistry(t *testing.T) {
	ctx := context.Background()
	renderer, manager := testPluginMarkdownRenderer(t, "mermaid", "tables")
	root := t.TempDir()
	source := filepath.Join(root, "docs")
	require.NoError(t, os.MkdirAll(source, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(source, "index.md"), []byte("```mermaid\ngraph LR; A --> B\n```"), 0644))
	assets := fstest.MapFS{}
	for _, name := range staticBrowserAssets {
		assets[name] = &fstest.MapFile{Data: []byte("asset")}
	}
	config := defaultConfig()
	config.SourceDir = source
	config.OutputDir = filepath.Join(root, "site")
	config.SiteURL = "https://example.com/docs/"
	require.NoError(t, BuildWithRenderer(ctx, assets, config, renderer))
	html, err := os.ReadFile(filepath.Join(config.OutputDir, "index.html"))
	require.NoError(t, err)
	assert.Contains(t, string(html), `id="kumbuka-plugin-modules"`)
	assert.Contains(t, string(html), "/docs/plugins/me.kumbuka.mermaid/")
	module := manager.BrowserModules()[0]
	frame, err := os.ReadFile(filepath.Join(config.OutputDir, "plugins", module.PluginID, module.Digest, "frames", "diagrams.html"))
	require.NoError(t, err)
	assert.Contains(t, string(frame), "/docs/assets/js/plugins/frame.js")
	assert.Contains(t, string(frame), "https://example.com/docs/plugins/")
	assert.NotContains(t, string(html), "modules.json")
	require.NoError(t, manager.Disable(ctx, module.PluginID))
	require.NoError(t, BuildWithRenderer(ctx, assets, config, renderer))
	html, err = os.ReadFile(filepath.Join(config.OutputDir, "index.html"))
	require.NoError(t, err)
	assert.NotContains(t, string(html), "me.kumbuka.mermaid")
	assert.Contains(t, string(html), "me.kumbuka.tables")
	_, err = os.Stat(filepath.Join(config.OutputDir, "plugins", module.PluginID))
	assert.True(t, os.IsNotExist(err))
	require.NoError(t, manager.Enable(ctx, module.PluginID))
}

func TestProjectRendererLoadsOnlyUsedDeclaredPlugins(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	filename := filepath.Join(root, ".kumbukaplugins")
	version := func(name string) string {
		archive, err := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		require.NoError(t, err)
		pkg, err := pluginpackage.Read(archive)
		require.NoError(t, err)
		return pkg.Manifest().Version
	}
	dependencies := fmt.Sprintf(`format = 1

[[plugin]]
id = "me.kumbuka.mermaid"
repository = "kumbuka-me/plugins"
tag_prefix = "mermaid/v"
asset = "mermaid"
version = %q

[[plugin]]
id = "me.kumbuka.tables"
repository = "kumbuka-me/plugins"
tag_prefix = "tables/v"
asset = "tables"
version = %q
`, version("mermaid"), version("tables"))
	require.NoError(t, os.WriteFile(filename, []byte(dependencies), 0o644))

	renderer, err := projectRenderer(ctx, filename, []sourcePage{{Markdown: "```mermaid\ngraph LR; A --> B\n```"}})
	require.NoError(t, err)
	defer func() { require.NoError(t, renderer.Close(context.Background())) }()

	loaded := renderer.PluginManager().Plugins()
	require.Len(t, loaded, 1)
	assert.Equal(t, "me.kumbuka.mermaid", loaded[0].Manifest.ID)
	assert.True(t, loaded[0].Enabled)
}
