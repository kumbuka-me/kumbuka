// Package pluginusage defines derived page metadata used to select source-aware
// plugin modules without making Markdown depend on persistence details.
package pluginusage

const Version = 1

// Index is a rebuildable summary of source-aware plugin modules used by one page. Markdown remains the source of truth; a nil index means the page has not been indexed.
type Index struct {
	// Version stores the version value used by index.
	Version int `json:"version"`
	// Fingerprint stores the fingerprint value used by index.
	Fingerprint string `json:"fingerprint"`
	// SourceHash stores the source hash value used by index.
	SourceHash string `json:"source_hash"`
	// Modules contains the modules associated with index.
	Modules []Module `json:"modules,omitempty"`
}

// Module records one source-aware module recognized in the page source.
type Module struct {
	// PluginID identifies the plugin associated with module.
	PluginID string `json:"plugin_id"`
	// ModuleID identifies the module associated with module.
	ModuleID string `json:"module_id"`
	// Values contains the values represented by module.
	Values []string `json:"values,omitempty"`
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
