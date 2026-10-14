package endpoint

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

// TestAssets verifies versioned assets are served from the embedded filesystem with immutable caching.
func TestAssets(t *testing.T) {
	t.Parallel()

	assets := fstest.MapFS{
		"css/app.css": &fstest.MapFile{Data: []byte("body{}")},
	}
	handler := Assets(assets)

	t.Run("serves versioned asset", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodGet, "/assets/v-abc123/css/app.css", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "body{}", response.Body.String())
		assert.Equal(t, "public, max-age=31536000, immutable", response.Header().Get("Cache-Control"))
	})

	t.Run("requires content version", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest(http.MethodGet, "/assets/css/app.css", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})
}

// TestAssetPath verifies the content-version path segment is removed only when present.
func TestAssetPath(t *testing.T) {
	t.Parallel()

	path, versioned := assetPath("/assets/v-deadbeef/js/main.js")

	assert.Equal(t, "js/main.js", path)
	assert.True(t, versioned)

	path, versioned = assetPath("/assets/js/main.js")

	assert.Empty(t, path)
	assert.False(t, versioned)
}

// TestAssetsCompressesVersionedBrowserFiles verifies shared gzip behavior retains immutable caching.
func TestAssetsCompressesVersionedBrowserFiles(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("body { background: white; }\n", 100)
	assets := fstest.MapFS{"css/app.css": &fstest.MapFile{Data: []byte(content)}}
	request := httptest.NewRequest(http.MethodGet, "/assets/v-abc123/css/app.css", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	response := httptest.NewRecorder()
	Assets(assets).ServeHTTP(response, request)
	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "gzip", response.Header().Get("Content-Encoding"))
	assert.Equal(t, "public, max-age=31536000, immutable", response.Header().Get("Cache-Control"))
	reader, err := gzip.NewReader(response.Body)
	assert.NoError(t, err)
	if err != nil {
		return
	}
	defer reader.Close() // nolint:errcheck
	decoded, err := io.ReadAll(reader)
	assert.NoError(t, err)
	assert.Equal(t, content, string(decoded))
}
