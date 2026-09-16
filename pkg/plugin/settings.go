package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// loadSettings returns persisted boolean settings for every settings module in manifest.
// Settings default to enabled when no value has been stored yet.
func (m *Manager) loadSettings(ctx context.Context, manifest pluginpackage.Manifest) (map[string]bool, error) {
	settings := make(map[string]bool)

	for _, module := range manifest.Modules {
		if module.Type != "settings" {
			continue
		}

		settings[module.ID] = true
		if m.values == nil {
			continue
		}

		value, found, err := m.values.ReadPluginValue(ctx, manifest.ID, "configuration", module.ID)
		if err != nil {
			return nil, fmt.Errorf("read plugin setting %s.%s: %w", manifest.ID, module.ID, err)
		}
		if !found {
			continue
		}

		var enabled bool
		if err := json.Unmarshal(value, &enabled); err != nil {
			return nil, fmt.Errorf("decode plugin setting %s.%s: %w", manifest.ID, module.ID, err)
		}
		settings[module.ID] = enabled
	}

	return settings, nil
}

// UpdateSettings persists the complete boolean settings state for one plugin.
func (m *Manager) UpdateSettings(ctx context.Context, id string, settings map[string]bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, err := m.find(id)
	if err != nil {
		return err
	}

	declared := make(map[string]pluginpackage.Module)
	for _, module := range item.metadata.Manifest.Modules {
		if module.Type == "settings" {
			declared[module.ID] = module
		}
	}
	if len(declared) == 0 {
		return errors.New("plugin does not expose settings")
	}
	if len(settings) != len(declared) {
		return errors.New("plugin settings are incomplete")
	}
	for key := range settings {
		if _, ok := declared[key]; !ok {
			return fmt.Errorf("unknown plugin setting %q", key)
		}
	}

	for id, module := range declared {
		if !settings[id] {
			continue
		}
		for _, dependency := range module.Requires {
			if required, ok := declared[dependency]; ok && !settings[required.ID] {
				return fmt.Errorf("%s requires %s to be enabled", module.Name, required.Name)
			}
		}
	}

	if m.values != nil {
		for key, enabled := range settings {
			value, err := json.Marshal(enabled)
			if err != nil {
				return err
			}
			if err := m.values.WritePluginValue(ctx, id, "configuration", key, value); err != nil {
				return fmt.Errorf("save plugin setting %s.%s: %w", id, key, err)
			}
		}
	}

	item.metadata.Settings = maps.Clone(settings)
	m.loaded[id] = item
	return nil
}

// FeatureSettings returns request feature flags contributed by enabled plugins.
func (m *Manager) FeatureSettings() map[string]bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	features := make(map[string]bool)
	for _, id := range m.order {
		item, ok := m.loaded[id]
		if !ok || !item.metadata.Enabled {
			continue
		}
		for key, enabled := range item.metadata.Settings {
			features[id+"."+key] = enabled
		}
	}

	return features
}
