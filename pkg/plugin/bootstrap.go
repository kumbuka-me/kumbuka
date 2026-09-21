package plugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Bootstrap merges embedded distribution bytes with durable overrides, checks the complete enabled dependency graph, and publishes once. Failed startup closes every prepared instance and leaves both persistence and registry alone.
func (m *Manager) Bootstrap(ctx context.Context, archives [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed || len(m.loaded) != 0 {
		return errors.New("plugin manager is not empty")
	}

	records, err := m.store.ListPlugins(ctx)
	if err != nil {
		return err
	}

	catalog, originals, err := m.bootstrapCatalog(ctx, archives, records)
	if err != nil {
		return err
	}
	if err := m.enableRequiredPlugins(catalog); err != nil {
		return err
	}

	order, err := dependencyOrder(catalog)
	if err != nil {
		return err
	}

	candidate, prepared, err := m.prepareBootstrapInstances(ctx, catalog, order)
	if err != nil {
		closePluginInstances(prepared)
		return err
	}
	if err := m.registry.initialize(candidate.entries); err != nil {
		closePluginInstances(prepared)
		return err
	}

	m.loaded = catalog
	m.order = order
	m.bundled = originals
	return nil
}

// bootstrapCatalog merges bundled archives with the persisted lifecycle state.
func (m *Manager) bootstrapCatalog(
	ctx context.Context,
	archives [][]byte,
	records []Record,
) (map[string]managedPlugin, map[string][]byte, error) {
	catalog := make(map[string]managedPlugin, len(archives)+len(records))
	originals := make(map[string][]byte, len(archives))

	for _, archive := range archives {
		item, err := m.managedPluginFromArchive(ctx, archive, SourceBundled)
		if err != nil {
			return nil, nil, err
		}

		id := item.metadata.Manifest.ID
		if _, exists := catalog[id]; exists {
			return nil, nil, fmt.Errorf("duplicate bundled plugin %s", id)
		}

		catalog[id] = item
		originals[id] = item.archive
	}

	seen := make(map[string]bool, len(records))
	for _, record := range records {
		if seen[record.ID] {
			return nil, nil, fmt.Errorf("duplicate stored plugin %s", record.ID)
		}
		seen[record.ID] = true

		if err := m.applyStoredPlugin(ctx, catalog, record); err != nil {
			return nil, nil, err
		}
	}

	return catalog, originals, nil
}

// managedPluginFromArchive validates package bytes and loads their persisted settings.
func (m *Manager) managedPluginFromArchive(
	ctx context.Context,
	archive []byte,
	source Source,
) (managedPlugin, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return managedPlugin{}, err
	}

	settings, err := m.loadSettings(ctx, pkg.Manifest())
	if err != nil {
		return managedPlugin{}, err
	}

	return managedPlugin{
		archive: bytes.Clone(archive),
		metadata: LoadedPlugin{
			Manifest: pkg.Manifest(),
			README:   pkg.README(),
			Settings: settings,
			Source:   source,
			Digest:   pkg.Digest(),
			Enabled:  pkg.Manifest().DefaultEnabled,
		},
	}, nil
}

// applyStoredPlugin overlays one persisted lifecycle record on the startup catalog.
func (m *Manager) applyStoredPlugin(
	ctx context.Context,
	catalog map[string]managedPlugin,
	record Record,
) error {
	switch record.Source {
	case SourceBundled:
		item, ok := catalog[record.ID]
		if !ok {
			return fmt.Errorf("stored bundled plugin %s is unavailable", record.ID)
		}
		if len(record.Package) != 0 {
			return errors.New("bundled state must not contain installed bytes")
		}

		item.metadata.Enabled = record.Enabled
		catalog[record.ID] = item
		return nil

	case SourceInstalled:
		// Installed bytes are an explicit administrator choice. Collapse the
		// override only when it is byte-identical to the bundled package.
		if bundled, ok := catalog[record.ID]; ok && sha256.Sum256(record.Package) == bundled.metadata.Digest {
			bundled.metadata.Enabled = record.Enabled
			catalog[record.ID] = bundled
			return nil
		}

		item, err := m.managedPluginFromArchive(ctx, record.Package, SourceInstalled)
		if err != nil {
			return err
		}
		if item.metadata.Manifest.ID != record.ID {
			return errors.New("stored plugin identity mismatch")
		}

		item.metadata.Enabled = record.Enabled
		catalog[record.ID] = item
		return nil

	default:
		return errors.New("invalid stored plugin source")
	}
}

// enableRequiredPlugins forces operator-required plugins on and checks their presence.
func (m *Manager) enableRequiredPlugins(catalog map[string]managedPlugin) error {
	for id := range m.required {
		item, ok := catalog[id]
		if !ok {
			return fmt.Errorf("required plugin %s is missing", id)
		}

		item.metadata.Enabled = true
		catalog[id] = item
	}

	return nil
}

// prepareBootstrapInstances loads enabled plugins into an unpublished candidate registry.
func (m *Manager) prepareBootstrapInstances(
	ctx context.Context,
	catalog map[string]managedPlugin,
	order []string,
) (*Registry, []Instance, error) {
	candidate := &Registry{}
	prepared := make([]Instance, 0, len(order))

	for _, id := range order {
		item := catalog[id]
		if !item.metadata.Enabled {
			continue
		}

		pkg, err := pluginpackage.Read(item.archive)
		if err != nil {
			return nil, prepared, err
		}

		instance, err := m.runtime.Load(ctx, pkg)
		if err != nil {
			return nil, prepared, fmt.Errorf("load plugin %s: %w", id, err)
		}
		prepared = append(prepared, instance)

		if err := candidate.Register(descriptor(item.metadata.Manifest), instance.Contributions()); err != nil {
			return nil, prepared, err
		}

		item.instance = instance
		catalog[id] = item
	}

	return candidate, prepared, nil
}

// closePluginInstances releases prepared instances after a failed bootstrap attempt.
func closePluginInstances(instances []Instance) {
	for _, instance := range instances {
		_ = instance.Close(context.Background())
	}
}

// dependencyOrder returns enabled plugins after their dependencies and rejects invalid graphs.
func dependencyOrder(catalog map[string]managedPlugin) ([]string, error) {
	names := make([]string, 0, len(catalog))
	for id := range catalog {
		names = append(names, id)
	}
	sort.Strings(names)

	state := make(map[string]int)
	var order []string
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 2 {
			return nil
		}
		if state[id] == 1 {
			return fmt.Errorf("plugin dependency cycle at %s", id)
		}
		state[id] = 1
		item := catalog[id]
		if item.metadata.Enabled {
			for _, dependency := range item.metadata.Manifest.Requires {
				required, ok := catalog[dependency]
				if !ok || !required.metadata.Enabled {
					return fmt.Errorf("plugin %s requires enabled plugin %s", id, dependency)
				}
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		state[id] = 2
		order = append(order, id)
		return nil
	}

	for _, id := range names {
		if err := visit(id); err != nil {
			return nil, err
		}
	}

	return order, nil
}
