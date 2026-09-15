package handler

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

type notFoundViewDataLoader struct{}

func (notFoundViewDataLoader) Load(_ *http.Request, _ *Views, title string) (ViewData, error) {
	return ViewData{Title: title}, nil
}

func TestNotFound(t *testing.T) {
	t.Parallel()

	page := template.Must(template.New("not_found").Parse(
		`{{ define "layout" }}<main><h1>{{ .Title }}</h1></main>{{ end }}`,
	))
	views := &Views{
		templates: map[string]*template.Template{"not_found": page},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	response := httptest.NewRecorder()

	NotFound(notFoundViewDataLoader{}, views).ServeHTTP(response, request)

	assert.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "Page not found")
}
