package pluginproject

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureArchive(t *testing.T, id, version string) []byte {
	t.Helper()
	var output bytes.Buffer
	writer := zip.NewWriter(&output)
	manifest := fmt.Sprintf(`api_version: 1
id: %s
name: Fixture
version: %s
default_enabled: true
modules:
  - type: markdown-syntax
    id: syntax
    syntax: strikethrough
    usage:
      - contains: "~~"
permissions: []
`, id, version)
	for name, data := range map[string][]byte{
		"README.md":   []byte("# Fixture\n"),
		"plugin.yaml": []byte(manifest),
	} {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	return output.Bytes()
}

func TestResolverDownloadsVerifiesAndCachesPackage(t *testing.T) {
	archive := fixtureArchive(t, "com.example.chart", "2.3.0")
	digest := sha256.Sum256(archive)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/example/chart/releases/download/v2.3.0/chart-2.3.0.kumbukaplugin":
			_, _ = w.Write(archive)
		case "/example/chart/releases/download/v2.3.0/chart-2.3.0.kumbukaplugin.sha256":
			_, _ = fmt.Fprintf(w, "%x  chart-2.3.0.kumbukaplugin\n", digest)
		default:
			http.NotFound(w, r)
		}
	}))

	resolver := &Resolver{
		CacheDir: filepath.Join(t.TempDir(), "cache"),
		Client:   server.Client(),
		baseURL:  server.URL,
	}
	dependency := Dependency{
		ID:         "com.example.chart",
		Repository: "example/chart",
		TagPrefix:  "v",
		Asset:      "chart",
		Version:    "2.3.0",
	}

	resolved, err := resolver.Resolve(context.Background(), []Dependency{dependency})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	assert.Equal(t, "com.example.chart", resolved[0].Manifest.ID)
	assert.Equal(t, 2, requests)

	server.Close()
	resolved, err = resolver.Resolve(context.Background(), []Dependency{dependency})
	require.NoError(t, err)
	require.Len(t, resolved, 1)
	assert.Equal(t, 2, requests)
}
