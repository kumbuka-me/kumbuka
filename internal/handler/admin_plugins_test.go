package handler

import (
	"bytes"
	"context"
	"html/template"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pluginUpload(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("package", "example.kumbukaplugin")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	r := httptest.NewRequest("POST", "/admin/plugins", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}
func TestAdminPluginLifecycleAndAuthorization(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithInterpreter())
	require.NoError(t, err)
	manager := plugin.NewManager(&plugin.Registry{}, runtime)
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	views, err := NewViews(web.Assets, slog.New(slog.NewTextHandler(io.Discard, nil)), "test", "test", nil, RuntimeInfo{})
	require.NoError(t, err)
	data := viewDataServiceStub{load: func(*http.Request, *Views, string) (ViewData, error) {
		return ViewData{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminPlugins(manager, data, views)
	archive, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	denied := httptest.NewRecorder()
	request := auth.WithUser(pluginUpload(t, archive), domain.User{ID: 2, Role: "viewer"})
	middleware.RequireRole("admin")(http.HandlerFunc(admin.Install)).ServeHTTP(denied, request)
	assert.Equal(t, http.StatusForbidden, denied.Code)
	assert.Empty(t, manager.Plugins())
	response := httptest.NewRecorder()
	admin.Install(response, pluginUpload(t, archive))
	require.Equal(t, http.StatusSeeOther, response.Code)
	require.Len(t, manager.Plugins(), 1)
	assert.True(t, manager.Plugins()[0].Enabled)
	id := manager.Plugins()[0].Manifest.ID
	for _, action := range []string{"disable", "enable"} {
		r := httptest.NewRequest("POST", "/admin/plugins/"+id+"/"+action, nil)
		r.SetPathValue("pluginID", id)
		r.SetPathValue("action", action)
		w := httptest.NewRecorder()
		admin.Action(w, r)
		require.Equal(t, http.StatusSeeOther, w.Code)
		assert.Equal(t, action == "enable", manager.Plugins()[0].Enabled)
	}
	detail := httptest.NewRequest("GET", "/admin/plugins?plugin="+id, nil)
	w := httptest.NewRecorder()
	admin.List(w, detail)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Callouts")
	assert.Contains(t, w.Body.String(), "data-plugin-detail-open-on-load")
	assert.Contains(t, w.Body.String(), "Permissions")
	assert.Contains(t, w.Body.String(), "Uninstall plugin")
	broken := pluginUpload(t, []byte("invalid archive"))
	broken.SetPathValue("pluginID", id)
	broken.SetPathValue("action", "upgrade")
	w = httptest.NewRecorder()
	admin.Action(w, broken)
	assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	assert.True(t, manager.Plugins()[0].Enabled)
	r := httptest.NewRequest("POST", "/admin/plugins/"+id+"/uninstall", nil)
	r.SetPathValue("pluginID", id)
	r.SetPathValue("action", "uninstall")
	w = httptest.NewRecorder()
	admin.Action(w, r)
	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Empty(t, manager.Plugins())
}
func TestPluginUploadBoundaries(t *testing.T) {
	_, status, err := readPluginUpload(httptest.NewRecorder(), httptest.NewRequest("POST", "/", bytes.NewBufferString("not multipart")))
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	_, status, err = readPluginUpload(httptest.NewRecorder(), pluginUpload(t, bytes.Repeat([]byte("x"), (16<<20)+1)))
	require.Error(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, status)
}

func TestAdminPluginMetadataIsEscaped(t *testing.T) {
	views, err := NewViews(web.Assets, slog.Default(), "test", "test", nil, RuntimeInfo{})
	require.NoError(t, err)
	data := ViewData{
		AdminPlugins:      []plugin.LoadedPlugin{{Manifest: pluginpackage.Manifest{ID: "io.example.safe", Name: "<script>bad()</script>", Provider: "<img src=x onerror=bad()>", Version: "1.0.0"}}},
		PluginRequiredIDs: make(map[string]bool),
		PluginHasSettings: make(map[string]bool),
		PluginREADMEs:     make(map[string]template.HTML),
	}
	html, err := renderTemplateHTML(views, "admin_plugins", "content", data)
	require.NoError(t, err)
	assert.NotContains(t, string(html), "<script>bad()")
	assert.NotContains(t, string(html), "<img src=x")
	assert.Contains(t, string(html), "&lt;script&gt;")
}
