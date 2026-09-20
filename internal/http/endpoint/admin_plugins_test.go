package endpoint

import (
	"bytes"
	"context"
	"errors"
	"html/template"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pluginUpdateServiceStub struct {
	updates       map[string]domain.PluginRelease
	archive       []byte
	updatesErr    error
	refreshErr    error
	downloadErr   error
	refreshes     int
	downloadedID  string
	downloadedVer string
	status        appplugins.PluginUpdateStatus
}

// Refresh supports plugin administration regression coverage.
func (s *pluginUpdateServiceStub) Refresh(context.Context) error {
	s.refreshes++
	return s.refreshErr
}

// Available returns the configured release set.
func (s *pluginUpdateServiceStub) Available() (map[string]domain.PluginRelease, error) {
	return s.updates, s.updatesErr
}

// Download records the requested release and returns the configured archive.
func (s *pluginUpdateServiceStub) Download(_ context.Context, id, version string) ([]byte, error) {
	s.downloadedID = id
	s.downloadedVer = version
	return s.archive, s.downloadErr
}

// Status supports plugin administration regression coverage.
func (s *pluginUpdateServiceStub) Status() appplugins.PluginUpdateStatus {
	return s.status
}

// pluginUpload supports plugin administration regression coverage.
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

// TestAdminPluginLifecycleAndAuthorization supports plugin administration regression coverage.
func TestAdminPluginLifecycleAndAuthorization(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithInterpreter())
	require.NoError(t, err)
	manager := plugin.NewManager(&plugin.Registry{}, runtime)
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminPlugins(manager, nil, data, views)
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
	assert.Contains(t, w.Body.String(), "Update checks are unavailable")
	assert.NotContains(t, w.Body.String(), "0001-01-01")
	assert.NotContains(t, w.Body.String(), "Version  is available")
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

// TestAdminPluginCatalogUpdate supports plugin administration regression coverage.
func TestAdminPluginCatalogUpdate(t *testing.T) {
	ctx := context.Background()
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithInterpreter())
	require.NoError(t, err)
	manager := plugin.NewManager(&plugin.Registry{}, runtime)
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	archive, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	item, err := manager.Install(ctx, archive)
	require.NoError(t, err)

	updates := &pluginUpdateServiceStub{
		status: appplugins.PluginUpdateStatus{
			Automatic:   true,
			LastAttempt: time.Date(2026, time.September, 18, 7, 31, 0, 0, time.UTC),
			LastSuccess: time.Date(2026, time.September, 18, 7, 31, 0, 0, time.UTC),
		},
		updates: map[string]domain.PluginRelease{
			item.Manifest.ID: {
				Version:    "9.9.9",
				ReleasedAt: time.Date(2026, time.September, 18, 7, 30, 0, 0, time.UTC),
			},
		},
		archive: archive,
	}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminPlugins(manager, updates, data, views)

	detail := httptest.NewRequest("GET", "/admin/plugins?plugin="+item.Manifest.ID, nil)
	w := httptest.NewRecorder()
	admin.List(w, detail)
	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "9.9.9 available")
	assert.Contains(t, w.Body.String(), "Update to 9.9.9")
	assert.Contains(t, w.Body.String(), "Published 2026-09-18")
	assert.NotContains(t, w.Body.String(), "0001-01-01")

	request := httptest.NewRequest("POST", "/admin/plugins/"+item.Manifest.ID+"/update?return=detail", nil)
	request.SetPathValue("pluginID", item.Manifest.ID)
	request.SetPathValue("action", "update")
	w = httptest.NewRecorder()
	admin.Action(w, request)

	require.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/admin/plugins?plugin="+item.Manifest.ID, w.Header().Get("Location"))
	assert.Equal(t, item.Manifest.ID, updates.downloadedID)
	assert.Equal(t, "9.9.9", updates.downloadedVer)
}

// TestAdminPluginManualCatalogRefresh supports plugin administration regression coverage.
func TestAdminPluginManualCatalogRefresh(t *testing.T) {
	updates := &pluginUpdateServiceStub{
		updates: map[string]domain.PluginRelease{},
		status:  appplugins.PluginUpdateStatus{Automatic: true},
	}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminPlugins(nil, updates, data, views)

	request := auth.WithUser(httptest.NewRequest("POST", "/admin/plugins/check-updates", nil), domain.User{ID: 1, Role: "admin"})
	w := httptest.NewRecorder()
	admin.CheckUpdates(w, request)

	require.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/admin/plugins", w.Header().Get("Location"))
	assert.Equal(t, 1, updates.refreshes)
}

// TestAdminPluginManualCatalogRefreshFailure supports plugin administration regression coverage.
func TestAdminPluginManualCatalogRefreshFailure(t *testing.T) {
	updates := &pluginUpdateServiceStub{
		updatesErr: errors.New("no cached catalog"),
		refreshErr: errors.New("catalog offline"),
		status: appplugins.PluginUpdateStatus{
			Automatic: true,
		},
	}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminPlugins(&plugin.Manager{}, updates, data, views)

	request := auth.WithUser(httptest.NewRequest("POST", "/admin/plugins/check-updates", nil), domain.User{ID: 1, Role: "admin"})
	w := httptest.NewRecorder()
	admin.CheckUpdates(w, request)

	assert.Equal(t, http.StatusBadGateway, w.Code)
	assert.Contains(t, w.Body.String(), "Could not check the plugin update catalog")
}

// TestPluginUploadBoundaries supports plugin administration regression coverage.
func TestPluginUploadBoundaries(t *testing.T) {
	_, status, err := readPluginUpload(httptest.NewRecorder(), httptest.NewRequest("POST", "/", bytes.NewBufferString("not multipart")))
	require.Error(t, err)
	assert.Equal(t, http.StatusBadRequest, status)
	_, status, err = readPluginUpload(httptest.NewRecorder(), pluginUpload(t, bytes.Repeat([]byte("x"), (16<<20)+1)))
	require.Error(t, err)
	assert.Equal(t, http.StatusRequestEntityTooLarge, status)
}

// TestAdminPluginMetadataIsEscaped supports plugin administration regression coverage.
func TestAdminPluginMetadataIsEscaped(t *testing.T) {
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := webview.AdminPluginsView{
		AdminPlugins:      []plugin.LoadedPlugin{{Manifest: pluginpackage.Manifest{ID: "io.example.safe", Name: "<script>bad()</script>", Provider: "<img src=x onerror=bad()>", Version: "1.0.0"}}},
		PluginRequiredIDs: make(map[string]bool),
		PluginHasSettings: make(map[string]bool),
		PluginREADMEs:     make(map[string]template.HTML),
	}
	html, err := views.RenderHTML("admin_plugins", "content", data)
	require.NoError(t, err)
	assert.NotContains(t, string(html), "<script>bad()")
	assert.NotContains(t, string(html), "<img src=x")
	assert.Contains(t, string(html), "&lt;script&gt;")
}
