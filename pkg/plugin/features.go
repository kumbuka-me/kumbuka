package plugin

import "fmt"

// ValidateFeatures checks request-scoped preferences against active declarations. Absent flags retain the default-enabled behavior used by public plugins.
func (s Snapshot) ValidateFeatures(features map[string]bool) error {
	for _, entry := range s.Entries {
		prefix := entry.Descriptor.ID + "."
		for _, module := range entry.Contributions.SettingsModules {
			if enabled, ok := features[prefix+module.ID]; ok && !enabled {
				continue
			}
			for _, dep := range module.Requires {
				if enabled, ok := features[prefix+dep]; ok && !enabled {
					return fmt.Errorf("%s requires %s to be enabled", module.Name, dep)
				}
			}
		}
	}
	return nil
}
