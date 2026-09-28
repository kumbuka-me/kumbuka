package endpoint

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
)

func TestPluginRenderStateChanged(t *testing.T) {
	t.Parallel()

	renderManifest := pluginpackage.Manifest{
		ID:      "rendering",
		Version: "1.0.0",
		Modules: []pluginpackage.Module{{Type: string(plugin.ModuleTypeMarkdownSyntax)}},
	}
	nonRenderManifest := pluginpackage.Manifest{
		ID:      "admin-only",
		Version: "1.0.0",
		Modules: []pluginpackage.Module{{Type: string(plugin.ModuleTypeAdminAction)}},
	}

	t.Run("render plugin version change", func(t *testing.T) {
		before := plugin.LoadedPlugin{Manifest: renderManifest, Enabled: true}
		after := before
		after.Manifest.Version = "1.1.0"
		assert.True(t, pluginRenderStateChanged(before, true, after, true))
	})

	t.Run("render plugin disabled", func(t *testing.T) {
		before := plugin.LoadedPlugin{Manifest: renderManifest, Enabled: true}
		after := before
		after.Enabled = false
		assert.True(t, pluginRenderStateChanged(before, true, after, true))
	})

	t.Run("render plugin installed enabled", func(t *testing.T) {
		after := plugin.LoadedPlugin{Manifest: renderManifest, Enabled: true}
		assert.True(t, pluginRenderStateChanged(plugin.LoadedPlugin{}, false, after, true))
	})

	t.Run("non rendering plugin version change", func(t *testing.T) {
		before := plugin.LoadedPlugin{Manifest: nonRenderManifest, Enabled: true}
		after := before
		after.Manifest.Version = "1.1.0"
		assert.False(t, pluginRenderStateChanged(before, true, after, true))
	})
}
