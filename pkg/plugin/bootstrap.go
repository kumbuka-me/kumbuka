package plugin

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Bootstrap reconciles offline distribution packages into the authoritative store,
// then eagerly prepares the complete enabled graph before publishing any registry.
func (m *Manager) Bootstrap(ctx context.Context, distribution Distribution) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || len(m.loaded) != 0 {
		return errors.New("plugin manager is not empty")
	}
	records, err := m.store.ListPlugins(ctx)
	if err != nil {
		return err
	}
	catalog, err := m.catalogFromRecords(ctx, records)
	if err != nil {
		return err
	}
	seeded := false
	if distribution != nil {
		seen := make(map[string]bool)
		for _, builtin := range distribution.Catalog() {
			if seen[builtin.ID] {
				return fmt.Errorf("duplicate builtin plugin %s", builtin.ID)
			}
			seen[builtin.ID] = true
			if _, exists := catalog[builtin.ID]; exists {
				continue
			}
			archive, err := distribution.Package(ctx, builtin.ID)
			if err != nil {
				return err
			}
			pkg, err := validateBuiltin(builtin, archive)
			if err != nil {
				return err
			}
			item, err := m.metadataFromPackage(ctx, pkg, pkg.Manifest().DefaultEnabled || m.required[builtin.ID])
			if err != nil {
				return err
			}
			if err := m.store.SeedPlugin(ctx, recordFor(item), archive); err != nil {
				return err
			}
			seeded = true
		}
	}
	// A concurrent startup may have inserted a different version or enabled
	// state after our first inventory read. Activate only the database winner.
	if seeded {
		records, err = m.store.ListPlugins(ctx)
		if err != nil {
			return err
		}
		catalog, err = m.catalogFromRecords(ctx, records)
		if err != nil {
			return err
		}
	}
	if err := m.enableRequiredPlugins(ctx, catalog); err != nil {
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
	m.loaded, m.order, m.distribution = catalog, order, distribution
	return nil
}

// catalogFromRecords restores runtime metadata exclusively from durable inventory.
func (m *Manager) catalogFromRecords(ctx context.Context, records []Record) (map[string]managedPlugin, error) {
	catalog := make(map[string]managedPlugin, len(records))
	for _, record := range records {
		if _, ok := catalog[record.ID]; ok {
			return nil, fmt.Errorf("duplicate stored plugin %s", record.ID)
		}
		if record.ID != record.Manifest.ID {
			return nil, errors.New("stored plugin identity mismatch")
		}
		settings, err := m.loadSettings(ctx, record.Manifest)
		if err != nil {
			return nil, err
		}
		catalog[record.ID] = managedPlugin{
			metadata: LoadedPlugin{
				Manifest: record.Manifest,
				Digest:   record.Digest,
				README:   record.README,
				Enabled:  record.Enabled,
				Settings: settings,
			},
		}
	}
	return catalog, nil
}

// metadataFromPackage returns a managed plugin from a package and its metadata.
func (m *Manager) metadataFromPackage(ctx context.Context, pkg *pluginpackage.Package, enabled bool) (managedPlugin, error) {
	settings, err := m.loadSettings(ctx, pkg.Manifest())
	if err != nil {
		return managedPlugin{}, err
	}
	widgets, problem := editorWidgetsFromPackage(pkg)
	return managedPlugin{
		metadata: LoadedPlugin{
			Manifest: pkg.Manifest(),
			README:   pkg.README(),
			Settings: settings,
			Digest:   pkg.Digest(),
			Enabled:  enabled,
		}, editorWidgets: widgets, editorWidgetProblem: problem,
	}, nil
}

// readPackage checks persisted bytes against the inventory identity on every use.
func (m *Manager) readPackage(ctx context.Context, item managedPlugin) (*pluginpackage.Package, error) {
	archive, err := m.store.PluginPackage(ctx, item.metadata.Manifest.ID)
	if err != nil {
		return nil, err
	}
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return nil, err
	}
	if !packageMatchesInventory(pkg, item.metadata) {
		return nil, errors.New("stored plugin package mismatch")
	}
	return pkg, nil
}

// packageMatchesInventory reports whether package-derived metadata still matches the persisted inventory.
func packageMatchesInventory(pkg *pluginpackage.Package, metadata LoadedPlugin) bool {
	return pkg.Digest() == metadata.Digest &&
		reflect.DeepEqual(pkg.Manifest(), metadata.Manifest)
}

// enableRequiredPlugins persists operator-required plugins as enabled and checks their presence.
func (m *Manager) enableRequiredPlugins(ctx context.Context, catalog map[string]managedPlugin) error {
	required := make([]string, 0, len(m.required))
	for id := range m.required {
		required = append(required, id)
	}
	sort.Strings(required)

	for _, id := range required {
		if _, ok := catalog[id]; !ok {
			return fmt.Errorf("required plugin %s is missing", id)
		}
	}

	for _, id := range required {
		item := catalog[id]
		if item.metadata.Enabled {
			continue
		}
		if err := m.store.SetPluginEnabled(ctx, id, true); err != nil {
			return err
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

		pkg, err := m.readPackage(ctx, item)
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

		item.editorWidgets, item.editorWidgetProblem = editorWidgetsFromPackage(pkg)
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
