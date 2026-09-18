package pluginupdate

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefreshPopulatesCatalogAndUpdatesSelectsNewestCompatibleRelease(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(catalog{
			SchemaVersion: catalogSchemaVersion,
			Plugins: []catalogPlugin{{
				ID: "me.kumbuka.callouts",
				Versions: []Release{
					catalogRelease(server.URL+"/callouts-2.0.0.kumbukaplugin", "2.0.0", int(sdk.Version)+1),
					catalogRelease(server.URL+"/callouts-1.2.0.kumbukaplugin", "1.2.0", int(sdk.Version)),
					catalogRelease(server.URL+"/callouts-1.1.0.kumbukaplugin", "1.1.0", int(sdk.Version)),
				},
			}},
		})
	}))
	defer server.Close()

	client := New(server.URL)
	require.NoError(t, client.Refresh(context.Background()))

	updates, err := client.Updates(map[string]string{"me.kumbuka.callouts": "1.0.0"})
	require.NoError(t, err)
	require.Contains(t, updates, "me.kumbuka.callouts")
	assert.Equal(t, "1.2.0", updates["me.kumbuka.callouts"].Version)

	updates, err = client.Updates(map[string]string{"me.kumbuka.callouts": "1.2.0"})
	require.NoError(t, err)
	assert.Empty(t, updates)
	assert.Equal(t, int32(1), requests.Load())
}

func TestUpdatesRequiresSuccessfulRefresh(t *testing.T) {
	t.Parallel()

	client := New("http://127.0.0.1/catalog.json")
	_, err := client.Updates(map[string]string{"me.kumbuka.callouts": "1.0.0"})

	require.ErrorIs(t, err, ErrCatalogUnavailable)
}

func TestRefreshFailurePreservesPreviousCatalog(t *testing.T) {
	t.Parallel()

	var fail atomic.Bool
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(catalog{
			SchemaVersion: catalogSchemaVersion,
			Plugins: []catalogPlugin{{
				ID: "me.kumbuka.callouts",
				Versions: []Release{
					catalogRelease(server.URL+"/callouts-1.2.0.kumbukaplugin", "1.2.0", int(sdk.Version)),
				},
			}},
		})
	}))
	defer server.Close()

	client := New(server.URL)
	require.NoError(t, client.Refresh(context.Background()))
	fail.Store(true)
	require.Error(t, client.Refresh(context.Background()))

	updates, err := client.Updates(map[string]string{"me.kumbuka.callouts": "1.0.0"})
	require.NoError(t, err)
	assert.Equal(t, "1.2.0", updates["me.kumbuka.callouts"].Version)
}

func TestNewUsesExplicitDefaultTransport(t *testing.T) {
	t.Parallel()

	client := New("https://kumbuka.me/plugins/catalog.json")

	transport, ok := client.httpClient.Transport.(*http.Transport)
	require.True(t, ok)
	assert.NotNil(t, transport.Proxy)
}

func TestDownloadUsesBoundedMemoryAndValidatesPackage(t *testing.T) {
	t.Parallel()

	archive := declarativePluginArchive(t, "me.kumbuka.callouts", "1.2.0")
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	client := New("http://127.0.0.1/catalog.json")
	release := Release{
		Version:    "1.2.0",
		APIVersion: int(sdk.Version),
		ReleasedAt: time.Now(),
		PackageURL: server.URL + "/callouts-1.2.0.kumbukaplugin",
		SHA256:     hex.EncodeToString(digest[:]),
	}

	downloaded, err := client.Download(context.Background(), "me.kumbuka.callouts", release)

	require.NoError(t, err)
	assert.Equal(t, archive, downloaded)
}

func TestDownloadRejectsChecksumMismatch(t *testing.T) {
	t.Parallel()

	archive := declarativePluginArchive(t, "me.kumbuka.callouts", "1.2.0")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	client := New("http://127.0.0.1/catalog.json")
	_, err := client.Download(context.Background(), "me.kumbuka.callouts", Release{
		Version:    "1.2.0",
		APIVersion: int(sdk.Version),
		ReleasedAt: time.Date(2026, time.September, 18, 7, 30, 0, 0, time.UTC),
		PackageURL: server.URL + "/callouts-1.2.0.kumbukaplugin",
		SHA256:     string(bytes.Repeat([]byte("0"), 64)),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "checksum")
}

func TestDownloadRejectsPackageIdentityMismatch(t *testing.T) {
	t.Parallel()

	archive := declarativePluginArchive(t, "me.kumbuka.other", "1.2.0")
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	client := New("http://127.0.0.1/catalog.json")
	_, err := client.Download(context.Background(), "me.kumbuka.callouts", Release{
		Version:    "1.2.0",
		APIVersion: int(sdk.Version),
		ReleasedAt: time.Date(2026, time.September, 18, 7, 30, 0, 0, time.UTC),
		PackageURL: server.URL + "/other-1.2.0.kumbukaplugin",
		SHA256:     hex.EncodeToString(digest[:]),
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "ID")
}

func catalogRelease(packageURL, version string, apiVersion int) Release {
	digest := sha256.Sum256([]byte(version))
	return Release{
		Version:    version,
		APIVersion: apiVersion,
		ReleasedAt: time.Date(2026, time.September, 18, 7, 30, 0, 0, time.UTC),
		PackageURL: packageURL,
		SHA256:     hex.EncodeToString(digest[:]),
	}
}

func declarativePluginArchive(t *testing.T, id, version string) []byte {
	t.Helper()

	var output bytes.Buffer
	archive := zip.NewWriter(&output)
	manifest := "api_version: 1\nprovider: Kumbuka\nid: " + id + "\nname: Fixture\nversion: " + version + "\ndescription: Fixture\ndefault_enabled: true\nmodules:\n  - type: markdown-syntax\n    id: fixture\n    syntax: strikethrough\npermissions: []\n"
	for name, content := range map[string]string{
		"plugin.yaml": manifest,
		"README.md":   "# Fixture\n",
	} {
		file, err := archive.Create(name)
		require.NoError(t, err)
		_, err = file.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, archive.Close())
	return output.Bytes()
}
