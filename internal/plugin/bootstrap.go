package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Bootstrap merges embedded distribution bytes with durable overrides, checks
// the complete enabled dependency graph, and publishes once. Failed startup
// closes every prepared instance and leaves both persistence and registry alone.
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

	catalog := make(map[string]managedPlugin)
	originals := make(map[string][]byte)

	for _, archive := range archives {
		pkg, err := pluginpackage.Read(archive)
		if err != nil {
			return err
		}
		id := pkg.Manifest().ID
		if _, ok := catalog[id]; ok {
			return fmt.Errorf("duplicate bundled plugin %s", id)
		}
		originals[id] = bytes.Clone(archive)
		settings, err := m.loadSettings(ctx, pkg.Manifest())
		if err != nil {
			return err
		}
		catalog[id] = managedPlugin{archive: originals[id], metadata: LoadedPlugin{Manifest: pkg.Manifest(), README: pkg.README(), Settings: settings, Source: SourceBundled, Digest: pkg.Digest(), Enabled: pkg.Manifest().DefaultEnabled}}
	}

	seen := make(map[string]bool)
	for _, record := range records {
		if seen[record.ID] {
			return fmt.Errorf("duplicate stored plugin %s", record.ID)
		}
		seen[record.ID] = true
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
		case SourceInstalled:
			pkg, err := pluginpackage.Read(record.Package)
			if err != nil {
				return err
			}
			if pkg.Manifest().ID != record.ID {
				return errors.New("stored plugin identity mismatch")
			}
			settings, err := m.loadSettings(ctx, pkg.Manifest())
			if err != nil {
				return err
			}
			catalog[record.ID] = managedPlugin{archive: bytes.Clone(record.Package), metadata: LoadedPlugin{Manifest: pkg.Manifest(), README: pkg.README(), Settings: settings, Source: SourceInstalled, Digest: pkg.Digest(), Enabled: record.Enabled}}
		default:
			return errors.New("invalid stored plugin source")
		}
	}

	for id := range m.required {
		item, ok := catalog[id]
		if !ok {
			return fmt.Errorf("required plugin %s is missing", id)
		}
		item.metadata.Enabled = true
		catalog[id] = item
	}
	order, err := dependencyOrder(catalog)
	if err != nil {
		return err
	}

	candidate := &Registry{}
	var prepared []Instance
	success := false
	defer func() {
		if !success {
			for _, instance := range prepared {
				_ = instance.Close(context.Background())
			}
		}
	}()

	for _, id := range order {
		item := catalog[id]
		if !item.metadata.Enabled {
			continue
		}
		pkg, err := pluginpackage.Read(item.archive)
		if err != nil {
			return err
		}
		instance, err := m.runtime.Load(ctx, pkg)
		if err != nil {
			return fmt.Errorf("load plugin %s: %w", id, err)
		}
		prepared = append(prepared, instance)
		if err := candidate.Register(descriptor(item.metadata.Manifest), instance.Contributions()); err != nil {
			return err
		}
		item.instance = instance
		catalog[id] = item
	}

	if err := m.registry.initialize(candidate.entries); err != nil {
		return err
	}
	m.loaded = catalog
	m.order = order
	m.bundled = originals
	success = true

	return nil
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
