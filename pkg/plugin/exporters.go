package plugin

import (
	"context"
	"fmt"
	"mime"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/kumbuka-me/sdk"
)

// ExporterContribution is one active page export format rendered by the host.
type ExporterContribution struct {
	// PluginID identifies the plugin that owns the exporter.
	PluginID string
	// ModuleID identifies the exporter module within the plugin.
	ModuleID string
	// Name is the human-readable export format name.
	Name string
	// Description is optional explanatory text shown by the host.
	Description string
	// Icon is the optional host icon rendered for the exporter.
	Icon string
	// URL is the host-owned endpoint that invokes this exporter for the current page.
	URL string
	// Order controls deterministic placement among plugin exporters.
	Order int
}

// Exporters returns active exporter metadata resolved for one current page.
func (m *Manager) Exporters(slug string) []ExporterContribution {
	var result []ExporterContribution
	for _, item := range m.Plugins() {
		if !item.Enabled {
			continue
		}
		for _, module := range item.Manifest.Modules {
			if ModuleType(module.Type) != ModuleTypeExporter {
				continue
			}
			result = append(result, ExporterContribution{
				PluginID: item.Manifest.ID, ModuleID: module.ID, Name: module.Name,
				Description: module.Description, Icon: module.Icon, Order: module.Order,
				URL: exporterURL(item.Manifest.ID, module.ID, slug),
			})
		}
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Order < result[j].Order })
	return result
}

// Export invokes one active exporter while pinning its plugin lifetime.
func (m *Manager) Export(ctx context.Context, pluginID, moduleID string, scope Context, request ExportRequest) (sdk.ExportFile, error) {
	entry, release, ok := m.registry.AcquireEntry(pluginID)
	if !ok {
		return sdk.ExportFile{}, fmt.Errorf("plugin %s is not active", pluginID)
	}
	defer release()

	for _, module := range entry.Contributions.Exporters {
		if module.ID != moduleID {
			continue
		}
		scope.Context = ctx
		file, err := Guard(pluginID, func() (sdk.ExportFile, error) { return module.Exporter.Export(scope, request) })
		if err != nil {
			return sdk.ExportFile{}, err
		}
		if !validExportFile(file) {
			return sdk.ExportFile{}, fmt.Errorf("plugin %s returned an invalid export file", pluginID)
		}
		return file, nil
	}
	return sdk.ExportFile{}, fmt.Errorf("exporter %s is not active in plugin %s", moduleID, pluginID)
}

// validExportFile validates host-facing filename, media type, and payload bounds.
func validExportFile(file sdk.ExportFile) bool {
	if !validExportFilename(file.Filename) {
		return false
	}
	if len(file.Data) > 4<<20 {
		return false
	}
	mediaType, _, err := mime.ParseMediaType(file.MediaType)
	return err == nil && mediaType != ""
}

// validExportFilename reports whether a plugin export filename is a safe single path component.
func validExportFilename(filename string) bool {
	if filename == "." || filename == ".." {
		return false
	}
	return len(filename) > 0 && len(filename) <= 255 && utf8.ValidString(filename) && !strings.ContainsAny(filename, "/\\\x00\r\n")
}

// exporterURL builds the host-owned invocation path for one exporter and page slug.
func exporterURL(pluginID, moduleID, slug string) string {
	parts := strings.Split(slug, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return "/export/plugin/" + url.PathEscape(pluginID) + "/" + url.PathEscape(moduleID) + "/" + strings.Join(parts, "/")
}
