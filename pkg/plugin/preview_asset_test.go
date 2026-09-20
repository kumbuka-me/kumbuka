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

func TestPluginPreviewServesDisabledPluginDocumentation(t *testing.T) {
	t.Parallel()

	preview := previewTestPNG(t)
	archive := previewTestArchive(t, preview, []byte("not public"))
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := previewTestManager(pkg, archive)
	data, err := manager.PluginPreview(pkg.Manifest().ID)

	require.NoError(t, err)
	assert.Equal(t, preview, data)
}

func TestBrowserAssetDoesNotExposeDisabledPluginPreview(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, previewTestPNG(t), []byte("not public"))
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := previewTestManager(pkg, archive)
	_, err = manager.BrowserAsset(pkg.Manifest().ID, "preview", pluginPreviewAsset)

	require.Error(t, err)
}

func TestPluginPreviewRejectsInvalidPNG(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, []byte("not a png"), nil)
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := previewTestManager(pkg, archive)
	_, err = manager.PluginPreview(pkg.Manifest().ID)

	require.Error(t, err)
}

func TestPluginPreviewReturnsNotFoundWhenPackageHasNoPreview(t *testing.T) {
	t.Parallel()

	archive := previewTestArchive(t, nil, nil)
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)

	manager := previewTestManager(pkg, archive)
	_, err = manager.PluginPreview(pkg.Manifest().ID)

	require.Error(t, err)
}

func previewTestManager(pkg *pluginpackage.Package, archive []byte) *Manager {
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
	return manager
}

func previewTestArchive(t *testing.T, preview, private []byte) []byte {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)

	files := map[string][]byte{
		"README.md":   []byte("# Preview fixture\n"),
		"plugin.yaml": []byte("api_version: 1\nid: io.example.preview\nname: Preview fixture\nversion: 1.0.0\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\npermissions: []\n"),
	}
	if preview != nil {
		files["assets/preview.png"] = preview
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
