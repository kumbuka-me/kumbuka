package handler

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/externalfiles"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/require"
)

type externalStore map[string][]byte

func (m externalStore) ReadPluginValue(_ context.Context, id, ns, k string) ([]byte, bool, error) {
	v, ok := m[id+ns+k]
	return v, ok, nil
}
func (m externalStore) ListPluginValues(context.Context, string, string, string) (map[string][]byte, error) {
	return m, nil
}
func (m externalStore) WritePluginValue(_ context.Context, id, ns, k string, v []byte) error {
	m[id+ns+k] = v
	return nil
}
func (m externalStore) DeletePluginValue(_ context.Context, id, ns, k string) error {
	delete(m, id+ns+k)
	return nil
}

func TestExternalSourceAdministration(t *testing.T) {
	cipher, err := secrets.New(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	require.NoError(t, err)
	store := externalStore{}
	sources := externalfiles.New(store, cipher)
	views := testHandlerViews(t, webview.RuntimeInfo{})
	data := viewDataServiceStub{load: func(*http.Request, *webview.Views, string) (webview.Data, error) {
		return webview.Data{User: domain.User{ID: 1, Role: "admin"}}, nil
	}}
	admin := NewAdminExternalFiles(sources, data, views)
	values := url.Values{"id": {"docs"}, "provider": {"github"}, "endpoint": {"https://api.github.com"}, "repository": {"team/docs"}, "ref": {"main"}, "token": {"private-token"}, "approve": {"on"}}
	request := func() *http.Request {
		r := httptest.NewRequest("POST", "/admin/plugins/external-files", strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return r
	}
	denied := httptest.NewRecorder()
	middleware.RequireRole("admin")(http.HandlerFunc(admin.Save)).ServeHTTP(denied, auth.WithUser(request(), domain.User{ID: 2, Role: "viewer"}))
	require.Equal(t, 403, denied.Code)
	require.Empty(t, store)
	values.Del("approve")
	w := httptest.NewRecorder()
	admin.Save(w, request())
	require.Equal(t, 400, w.Code)
	require.Empty(t, store)
	values.Set("approve", "on")
	w = httptest.NewRecorder()
	admin.Save(w, request())
	require.Equal(t, 303, w.Code)
	require.Len(t, store, 1)
	w = httptest.NewRecorder()
	admin.List(w, httptest.NewRequest("GET", "/admin/plugins/external-files", nil))
	require.Equal(t, 200, w.Code)
	require.Contains(t, w.Body.String(), "team/docs")
	require.Contains(t, w.Body.String(), "Configured")
	require.NotContains(t, w.Body.String(), "private-token")
	require.Equal(t, "private, no-store", w.Header().Get("Cache-Control"))
	if output := os.Getenv("KUMBUKA_EXTERNAL_ADMIN_TEST_HTML"); output != "" {
		require.NoError(t, os.WriteFile(output, w.Body.Bytes(), 0600))
	}
	r := httptest.NewRequest("POST", "/admin/plugins/external-files/docs/delete", nil)
	r.SetPathValue("source", "docs")
	w = httptest.NewRecorder()
	admin.Delete(w, r)
	require.Equal(t, 303, w.Code)
	require.Empty(t, store)
}
