package plugin

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserAssetServesPreviewForDisabledPlugin(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, previewTestPNG(t), []byte("not public"))
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := NewManager(&Registry{}, nil)
	manager.loaded[pkg.Manifest().ID] = managedPlugin{
		archive: archive,
		metadata: LoadedPlugin{
			Enabled:  false,
			Manifest: pkg.Manifest(),
			Digest:   pkg.Digest(),
		},
	}
	manager.order = []string{pkg.Manifest().ID}

	data, err := manager.BrowserAsset(pkg.Manifest().ID, pluginPreviewDigest, pluginPreviewAsset)

	require.NoError(t, err)
	assert.Equal(t, previewTestPNG(t), data)
}

func TestBrowserAssetKeepsOtherDisabledAssetsPrivate(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, previewTestPNG(t), []byte("not public"))
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := NewManager(&Registry{}, nil)
	manager.loaded[pkg.Manifest().ID] = managedPlugin{
		archive: archive,
		metadata: LoadedPlugin{
			Enabled:  false,
			Manifest: pkg.Manifest(),
			Digest:   pkg.Digest(),
		},
	}
	manager.order = []string{pkg.Manifest().ID}

	_, err = manager.BrowserAsset(pkg.Manifest().ID, pluginPreviewDigest, "private.txt")

	require.Error(t, err)
}

func TestBrowserAssetRejectsInvalidPreviewPNG(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, []byte("not a png"), nil)
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := NewManager(&Registry{}, nil)
	manager.loaded[pkg.Manifest().ID] = managedPlugin{
		archive: archive,
		metadata: LoadedPlugin{
			Enabled:  false,
			Manifest: pkg.Manifest(),
			Digest:   pkg.Digest(),
		},
	}
	manager.order = []string{pkg.Manifest().ID}

	_, err = manager.BrowserAsset(pkg.Manifest().ID, pluginPreviewDigest, pluginPreviewAsset)

	require.Error(t, err)
}

func previewTestArchive(t *testing.T, preview, private []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	files := map[string][]byte{
		"README.md":          []byte("# Preview fixture\n"),
		"plugin.yaml":        []byte("api_version: 1\nid: io.example.preview\nname: Preview fixture\nversion: 1.0.0\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n"),
		"assets/preview.png": preview,
	}
	if private != nil {
		files["assets/private.txt"] = private
	}

	for name, data := range files {
		entry, err := writer.Create(name)
		require.NoError(t, err)
		_, err = entry.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	return buffer.Bytes()
}

func previewTestPNG(t *testing.T) []byte {
	t.Helper()

	var buffer bytes.Buffer
	require.NoError(t, png.Encode(&buffer, image.NewRGBA(image.Rect(0, 0, 32, 18))))
	return buffer.Bytes()
}
