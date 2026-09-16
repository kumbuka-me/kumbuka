package plugin

import (
	"context"
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
)

type testExporter struct{}

func (testExporter) Export(_ Context, request ExportRequest) (sdk.ExportFile, error) {
	return sdk.ExportFile{Filename: request.Page.Slug + ".txt", MediaType: "text/plain", Data: []byte(request.Source)}, nil
}

func TestManagerExportersAndInvocation(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register(Descriptor{ID: "example.export", Name: "Export"}, Contributions{
		Exporters: []ExporterModule{{ID: "text", Name: "Text", Order: 20, Exporter: testExporter{}}},
	}))
	manager := &Manager{registry: registry, loaded: map[string]managedPlugin{
		"example.export": {metadata: LoadedPlugin{Enabled: true, Manifest: pluginManifestWithExporter()}},
	}, order: []string{"example.export"}}

	items := manager.Exporters("guide/start")
	require.Len(t, items, 1)
	require.Equal(t, "/export/plugin/example.export/text/guide/start", items[0].URL)

	file, err := manager.Export(context.Background(), "example.export", "text", Context{}, ExportRequest{Page: sdk.Page{Slug: "guide"}, Source: "# Guide"})
	require.NoError(t, err)
	require.Equal(t, "guide.txt", file.Filename)
	require.Equal(t, []byte("# Guide"), file.Data)
}

func pluginManifestWithExporter() pluginpackage.Manifest {
	return pluginpackage.Manifest{ID: "example.export", Modules: []pluginpackage.Module{{Type: "exporter", ID: "text", Name: "Text", Order: 20}}}
}
