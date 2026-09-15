// Package pluginusage defines derived page metadata used to select source-aware
// plugin modules without making Markdown depend on persistence details.
package pluginusage

const Version = 1

// Index is a rebuildable summary of source-aware plugin modules used by one page.
// Markdown remains the source of truth; a nil index means the page has not been indexed.
type Index struct {
	Version     int      `json:"version"`
	Fingerprint string   `json:"fingerprint"`
	SourceHash  string   `json:"source_hash"`
	Modules     []Module `json:"modules,omitempty"`
}

// Module records one source-aware module recognized in the page source.
type Module struct {
	PluginID string   `json:"plugin_id"`
	ModuleID string   `json:"module_id"`
	Values   []string `json:"values,omitempty"`
}

// Has reports whether the index selected one plugin module.
func (i Index) Has(pluginID, moduleID string) bool {
	for _, module := range i.Modules {
		if module.PluginID == pluginID && module.ModuleID == moduleID {
			return true
		}
	}
	return false
}
