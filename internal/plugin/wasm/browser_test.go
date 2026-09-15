package wasm_test

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstalledBrowserPluginUsesSameRuntimeAndAssets(t *testing.T) {
	ctx := context.Background()
	archive, err := plugins.Packages.ReadFile("mermaid.kumbukaplugin")
	require.NoError(t, err)
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	denied, err := wasm.New(ctx, wasm.Limits{})
	require.NoError(t, err)
	_, err = denied.Load(ctx, pkg)
	require.ErrorContains(t, err, "browser:render")
	require.NoError(t, denied.Close(ctx))
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("browser:render"))
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	_, err = manager.Install(ctx, archive)
	require.NoError(t, err)
	renderer := markdown.NewWithRegistry(registry)
	html, err := renderer.Render("```mermaid\ngraph LR; A --> B\n```")
	require.NoError(t, err)
	assert.Contains(t, html, `data-kumbuka-plugin="me.kumbuka.mermaid"`)
	modules := manager.BrowserModules()
	require.Len(t, modules, 1)
	old := modules[0]
	actual, err := manager.BrowserAsset(old.PluginID, old.Digest, "plugin.js")
	require.NoError(t, err)
	expected, err := pkg.Asset("plugin.js")
	require.NoError(t, err)
	assert.Equal(t, expected, actual)
	replacement := changedManifestVersion(t, archive, "1.1.0")
	_, err = manager.Upgrade(ctx, old.PluginID, replacement)
	require.NoError(t, err)
	_, err = manager.BrowserAsset(old.PluginID, old.Digest, "plugin.js")
	require.Error(t, err)
	next := manager.BrowserModules()[0]
	assert.Equal(t, "1.1.0", next.Version)
	assert.NotEqual(t, old.Digest, next.Digest)
	_, err = manager.BrowserAsset(next.PluginID, next.Digest, "plugin.js")
	require.NoError(t, err)
	require.NoError(t, manager.Uninstall(ctx, next.PluginID))
	assert.Empty(t, manager.BrowserModules())
	_, err = manager.BrowserAsset(next.PluginID, next.Digest, "plugin.js")
	require.Error(t, err)
}

func TestTablesPackageOwnsSyntaxAndPresentation(t *testing.T) {
	ctx := context.Background()
	archive, err := plugins.Packages.ReadFile("tables.kumbukaplugin")
	require.NoError(t, err)
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("browser:render"))
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	renderer := markdown.NewWithRegistry(registry)
	source := "| Service | Link |\n| --- | --- |\n| API | [[Runbook]] |\n\n{table header=blue sortable filterable}\n"
	_, err = manager.Install(ctx, archive)
	require.NoError(t, err)
	html, err := renderer.Render(source)
	require.NoError(t, err)
	assert.Contains(t, html, `data-kumbuka-plugin="me.kumbuka.tables"`)
	assert.Contains(t, html, `data-kumbuka-fallback`)
	assert.Contains(t, html, `href="/pages/runbook"`)
	assert.Contains(t, html, `table-tone-blue`)
	assert.Contains(t, html, `kumbuka-table-sortable`)
	assert.NotContains(t, html, "{table")
	require.Error(t, registry.Snapshot().ValidateFeatures(map[string]bool{"me.kumbuka.tables.tables": false}))
	require.NoError(t, manager.Disable(ctx, "me.kumbuka.tables"))
	html, err = renderer.Render(source)
	require.NoError(t, err)
	assert.NotContains(t, html, "<table")
	assert.Empty(t, manager.BrowserModules())
	require.NoError(t, manager.Enable(ctx, "me.kumbuka.tables"))
	html, err = renderer.Render(source)
	require.NoError(t, err)
	assert.Contains(t, html, "<table")
	require.NoError(t, manager.Uninstall(ctx, "me.kumbuka.tables"))
	html, err = renderer.Render(source)
	require.NoError(t, err)
	assert.NotContains(t, html, "<table")
}
