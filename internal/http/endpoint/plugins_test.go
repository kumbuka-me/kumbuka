package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/pkg/pluginbrowser"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginPresentationStylesCaching(t *testing.T) {
	t.Parallel()

	version := pluginbrowser.PresentationStylesVersion(nil)
	handler := PluginPresentationStyles(nil)

	t.Run("unversioned stays uncached", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", "/plugins/styles.css", nil))

		assert.Equal(t, 200, response.Code)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	})

	t.Run("current version is immutable", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", "/plugins/styles.css?v="+version, nil))

		assert.Equal(t, 200, response.Code)
		assert.Equal(t, "public, max-age=31536000, immutable", response.Header().Get("Cache-Control"))
	})

	t.Run("stale version is rejected", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		handler(response, httptest.NewRequest("GET", "/plugins/styles.css?v=stale", nil))

		assert.Equal(t, 404, response.Code)
		assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	})
}

func TestPluginPreviewAllowsSameOriginEmbedding(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/plugins/missing/preview.png", nil)

	PluginPreview(nil)(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "SAMEORIGIN", response.Header().Get("X-Frame-Options"))
	assert.Equal(t, "default-src 'none'; sandbox; frame-ancestors 'self'", response.Header().Get("Content-Security-Policy"))
}

func TestBrowserPluginLifecycleAndAssetBoundary(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithPermissions("browser:render"), wasm.WithInterpreter())
	require.NoError(t, err)
	manager := plugin.NewManager(&plugin.Registry{}, runtime)
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })
	archives := make([][]byte, 0, 2)
	for _, name := range []string{"mermaid", "tables"} {
		archive, err := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		require.NoError(t, err)
		archives = append(archives, archive)
	}
	require.NoError(t, manager.Bootstrap(ctx, archives))
	modules := pluginbrowser.Catalog("/plugins", manager)
	require.Len(t, modules, 2)
	module := modules[0]
	assert.Equal(t, "me.kumbuka.mermaid", module.PluginID)
	asset := func(name, digest string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "http://kumbuka.test/asset", nil)
		r.SetPathValue("pluginID", module.PluginID)
		r.SetPathValue("digest", digest)
		r.SetPathValue("asset", name)
		w := httptest.NewRecorder()
		PluginAssets(manager)(w, r)
		return w
	}
	w := asset("plugin.js", module.Digest)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Body.String(), "kumbukaPlugin")
	assert.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	for _, name := range []string{"../plugin.wasm", "/plugin.js", "..\\plugin.js", "missing.js", "a/../plugin.js"} {
		assert.Equal(t, 404, asset(name, module.Digest).Code, name)
	}
	assert.Equal(t, 404, asset("plugin.js", "stale").Code)
	r := httptest.NewRequest("GET", "http://kumbuka.test/", nil)
	r.SetPathValue("pluginID", module.PluginID)
	r.SetPathValue("digest", module.Digest)
	r.SetPathValue("frame", module.ModuleID+".html")
	w = httptest.NewRecorder()
	PluginFrame(manager)(w, r)
	require.Equal(t, 200, w.Code)
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "sandbox allow-scripts")
	assert.Contains(t, w.Header().Get("Content-Security-Policy"), "connect-src 'none'")
	assert.NotContains(t, w.Header().Get("Content-Security-Policy"), "allow-same-origin")
	assert.Contains(t, w.Body.String(), "/plugins/runtime.js")
	require.NoError(t, manager.Disable(ctx, module.PluginID))
	assert.Len(t, manager.BrowserModules(), 1)
	assert.Equal(t, 404, asset("plugin.js", module.Digest).Code)
	w = httptest.NewRecorder()
	PluginFrame(manager)(w, r)
	assert.Equal(t, 404, w.Code)
	require.NoError(t, manager.Enable(ctx, module.PluginID))
	assert.Equal(t, 200, asset("plugin.js", module.Digest).Code)
}
