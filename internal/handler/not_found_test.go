package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/stretchr/testify/assert"
)

type notFoundViewDataLoader struct{}

func (notFoundViewDataLoader) Load(_ *http.Request, _ *webview.Views, title string) (webview.Data, error) {
	return webview.Data{Title: title}, nil
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	views := testHandlerViewsWithOverrides(
		t,
		testViewsLogger(),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/layout.gohtml": `{{ define "layout" }}<main><h1>{{ .Title }}</h1></main>{{ end }}`,
		},
	)
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()

	NotFound(notFoundViewDataLoader{}, views).ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "Page not found")
}
