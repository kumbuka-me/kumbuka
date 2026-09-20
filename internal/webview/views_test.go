package webview

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("parses page templates and keeps view metadata", func(t *testing.T) {
		t.Parallel()

		logger := testViewsLogger()
		runtime := RuntimeInfo{
			ListenAddress: "127.0.0.1:8080",
			PublicURL:     "https://kumbuka.example.test",
		}

		views, err := New(
			testViewFS(),
			logger,
			"v1.2.3",
			"abc123",
			nil,
			runtime,
		)

		require.NoError(t, err)
		assert.Len(t, views.templates, len(pageTemplateNames))
		assert.Same(t, logger, views.logger)
		assert.Equal(t, "v1.2.3", views.version)
		assert.Equal(t, "abc123", views.commit)
		assert.Equal(t, runtime, views.runtime)
		assert.Len(t, views.assetVersion, 16)
	})

	t.Run("does not require the legacy horizontal logo", func(t *testing.T) {
		t.Parallel()

		views, err := New(
			testViewFS(),
			testViewsLogger(),
			"dev",
			"none",
			nil,
			RuntimeInfo{},
		)

		require.NoError(t, err)
		assert.NotNil(t, views)
	})

	t.Run("requires shared template files", func(t *testing.T) {
		t.Parallel()

		appFS := testViewFS()
		delete(appFS, "templates/header.gohtml")

		views, err := New(
			appFS,
			testViewsLogger(),
			"dev",
			"none",
			nil,
			RuntimeInfo{},
		)

		assert.Nil(t, views)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse login template")
	})

	t.Run("rejects invalid page template", func(t *testing.T) {
		t.Parallel()

		appFS := testViewFS()
		appFS["templates/login.gohtml"] = &fstest.MapFile{Data: []byte("{{")}

		views, err := New(
			appFS,
			testViewsLogger(),
			"dev",
			"none",
			nil,
			RuntimeInfo{},
		)

		assert.Nil(t, views)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse login template")
	})
}

func TestRender(t *testing.T) {
	t.Parallel()

	t.Run("renders layout", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "layout" }}layout: {{ .Title }}{{ end }}`)
		response := httptest.NewRecorder()

		views.Render(response, "page", Layout{Title: "Example"})

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
		assert.Equal(t, "layout: Example", response.Body.String())
	})

	t.Run("renders explicit status", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "layout" }}not found{{ end }}`)
		response := httptest.NewRecorder()

		views.RenderStatus(response, http.StatusNotFound, "page", Layout{})

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
		assert.Equal(t, "not found", response.Body.String())
	})

	t.Run("renders public layout", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "public-layout" }}public: {{ .Title }}{{ end }}`)
		response := httptest.NewRecorder()

		views.RenderPublic(response, "page", Layout{Title: "Login"})

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "public: Login", response.Body.String())
	})

	t.Run("renders named fragment", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "fragment" }}fragment: {{ .Title }}{{ end }}`)
		response := httptest.NewRecorder()

		views.RenderFragment(response, "page", "fragment", Layout{Title: "Navigation"})

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "fragment: Navigation", response.Body.String())
	})
}

func TestRenderTemplateStatus(t *testing.T) {
	t.Parallel()

	t.Run("returns internal server error for unknown page", func(t *testing.T) {
		t.Parallel()

		views := (&Views{
			templates: map[string]*template.Template{},
			logger:    testViewsLogger(),
		}).WithRenderErrorHandler(testRenderErrorHandler)
		response := httptest.NewRecorder()

		views.RenderDataStatus(response, http.StatusOK, "missing", "layout", Layout{})

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, response.Body.String(), "The request could not be processed")
	})

	t.Run("returns internal server error without partial output on execution failure", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "layout" }}prefix{{ .UnknownField }}{{ end }}`)
		response := httptest.NewRecorder()

		views.RenderDataStatus(response, http.StatusOK, "page", "layout", Layout{})

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.NotContains(t, response.Body.String(), "prefix")
		assert.Contains(t, response.Body.String(), "The request could not be processed")
	})
}

func TestRenderErrorHandlerIsInjectedByAdapter(t *testing.T) {
	t.Parallel()

	views := &Views{templates: map[string]*template.Template{}, logger: testViewsLogger()}
	called := false
	views.WithRenderErrorHandler(func(logger *slog.Logger, w http.ResponseWriter, err error) {
		called = true
		assert.NotNil(t, logger)
		assert.Error(t, err)
		http.Error(w, "adapter error", http.StatusInternalServerError)
	})
	response := httptest.NewRecorder()

	views.RenderDataStatus(response, http.StatusOK, "missing", "layout", Layout{})

	assert.True(t, called)
	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Contains(t, response.Body.String(), "adapter error")
}

func TestRenderTemplateHTML(t *testing.T) {
	t.Parallel()

	t.Run("renders trusted fragment", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "fragment" }}<strong>{{ .Title }}</strong>{{ end }}`)

		html, err := views.RenderHTML("page", "fragment", Layout{Title: "Example"})

		require.NoError(t, err)
		assert.Equal(t, template.HTML("<strong>Example</strong>"), html)
	})

	t.Run("escapes ordinary template values", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "fragment" }}{{ .Title }}{{ end }}`)

		html, err := views.RenderHTML("page", "fragment", Layout{Title: "<script>alert(1)</script>"})

		require.NoError(t, err)
		assert.Equal(t, template.HTML("&lt;script&gt;alert(1)&lt;/script&gt;"), html)
	})

	t.Run("returns error for unknown page", func(t *testing.T) {
		t.Parallel()

		views := (&Views{
			templates: map[string]*template.Template{},
			logger:    testViewsLogger(),
		}).WithRenderErrorHandler(testRenderErrorHandler)

		html, err := views.RenderHTML("missing", "fragment", Layout{})

		assert.Empty(t, html)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `page template "missing" not found`)
	})

	t.Run("wraps template execution error", func(t *testing.T) {
		t.Parallel()

		views := testViewsWithTemplate(`{{ define "fragment" }}{{ .UnknownField }}{{ end }}`)

		html, err := views.RenderHTML("page", "fragment", Layout{})

		assert.Empty(t, html)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "render page template fragment")
	})
}

func testViewFS() fstest.MapFS {
	appFS := fstest.MapFS{}

	for _, filename := range sharedTemplateFiles {
		appFS[filename] = &fstest.MapFile{}
	}

	appFS["templates/layout.gohtml"] = &fstest.MapFile{Data: []byte(`
		{{ define "layout" }}
			{{ template "content" . }}
		{{ end }}
	`)}
	appFS["templates/public_layout.gohtml"] = &fstest.MapFile{Data: []byte(`
		{{ define "public-layout" }}
			{{ template "content" . }}
		{{ end }}
	`)}

	for _, name := range pageTemplateNames {
		appFS["templates/"+name+".gohtml"] = &fstest.MapFile{Data: []byte(`
			{{ define "content" }}
				{{ .Title }}
			{{ end }}
		`)}
	}

	return appFS
}

func testViewsWithTemplate(source string) *Views {
	return (&Views{
		templates: map[string]*template.Template{
			"page": template.Must(template.New("page").Parse(source)),
		},
		logger: testViewsLogger(),
	}).WithRenderErrorHandler(testRenderErrorHandler)
}

func testRenderErrorHandler(_ *slog.Logger, w http.ResponseWriter, _ error) {
	http.Error(w, "The request could not be processed.", http.StatusInternalServerError)
}

func testViewsLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
