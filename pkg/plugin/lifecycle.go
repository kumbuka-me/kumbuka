package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// retirement tracks asynchronous shutdown of one replaced plugin instance.
type retirement struct {
	// done signals that the asynchronous operation has completed.
	done chan struct{}
	// err stores the terminal operation error.
	err error
}

// retire closes an instance once every render lease on its contribution version drains.
func (m *Manager) retire(life *lifetime, instance Instance) {
	// Drop completed successful retirements to keep long-running managers bounded.
	m.retirements = slices.DeleteFunc(m.retirements, func(r *retirement) bool {
		select {
		case <-r.done:
			return r.err == nil
		default:
			return false
		}
	})

	retired := &retirement{done: make(chan struct{})}
	m.retirements = append(m.retirements, retired)

	closeInstance := func() { retired.err = instance.Close(context.Background()); close(retired.done) }
	if life == nil {
		closeInstance()
		return
	}

	drained := life.retire()
	select {
	case <-drained:
		closeInstance()
	default:
		go func() { <-drained; closeInstance() }()
	}
}

// descriptor converts package manifest metadata into registry metadata.
func descriptor(manifest pluginpackage.Manifest) Descriptor {
	return Descriptor{
		ID:             manifest.ID,
		Name:           manifest.Name,
		Description:    manifest.Description,
		DefaultEnabled: manifest.DefaultEnabled,
		Requires:       manifest.Requires,
	}
}

// recordFor converts managed runtime state into durable installation state.
func recordFor(item managedPlugin) Record {
	return Record{
		ID:       item.metadata.Manifest.ID,
		Manifest: item.metadata.Manifest,
		Digest:   item.metadata.Digest,
		README:   item.metadata.README,
		Enabled:  item.metadata.Enabled,
	}
}

// Install validates and starts a package before durable, atomic publication.
func (m *Manager) Install(ctx context.Context, archive []byte) (LoadedPlugin, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return LoadedPlugin{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return LoadedPlugin{}, errors.New("plugin manager is closed")
	}
	id := pkg.Manifest().ID
	if _, ok := m.loaded[id]; ok {
		return LoadedPlugin{}, fmt.Errorf("plugin %s is already installed", id)
	}

	item, err := m.prepare(ctx, pkg, true)
	if err != nil {
		return LoadedPlugin{}, err
	}
	if err = m.publish(ctx, id, item, archive); err != nil {
		return LoadedPlugin{}, err
	}

	return m.describe(item.metadata), nil
}

// prepare validates and instantiates a package before any durable or registry change.
func (m *Manager) prepare(ctx context.Context, pkg *pluginpackage.Package, enabled bool) (managedPlugin, error) {
	if m.required[pkg.Manifest().ID] {
		enabled = true
	}
	item, err := m.metadataFromPackage(ctx, pkg, enabled)
	if err != nil {
		return managedPlugin{}, err
	}
	if enabled {
		item.instance, err = m.runtime.Load(ctx, pkg)
		if err != nil {
			return managedPlugin{}, err
		}
	}
	return item, nil
}

// publish commits durable state and atomically transitions the active registry entry.
func (m *Manager) publish(ctx context.Context, id string, item managedPlugin, archive []byte) error {
	previous := m.loaded[id]
	catalog := make(map[string]managedPlugin, len(m.loaded)+1)
	for key, value := range m.loaded {
		catalog[key] = value
	}
	catalog[id] = item
	order, graphErr := dependencyOrder(catalog)
	if graphErr != nil {
		if item.instance != nil {
			_ = item.instance.Close(context.Background())
		}
		return graphErr
	}
	commit := func() error {
		if archive == nil {
			return m.store.SetPluginEnabled(ctx, id, item.metadata.Enabled)
		}
		return m.store.SavePlugin(ctx, recordFor(item), archive)
	}

	var old *lifetime
	var err error

	if item.instance != nil {
		entry := &Entry{Descriptor: descriptor(item.metadata.Manifest), Contributions: item.instance.Contributions()}
		old, err = m.registry.transition(id, entry, previous.instance != nil, commit)
	} else if previous.instance != nil {
		old, err = m.registry.transition(id, nil, true, commit)
	} else {
		err = commit()
	}

	if err != nil {
		if item.instance != nil {
			_ = item.instance.Close(context.Background())
		}
		return err
	}

	m.loaded[id] = item
	m.order = order
	if previous.instance != nil {
		m.retire(old, previous.instance)
	}

	return nil
}

// Enable validates and activates an installed plugin without restarting Kumbuka.
func (m *Manager) Enable(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	item, err := m.find(id)
	if err != nil {
		return err
	}
	if item.metadata.Enabled {
		return nil
	}

	pkg, err := m.readPackage(ctx, item)
	if err != nil {
		return err
	}

	prepared, err := m.prepare(ctx, pkg, true)
	if err != nil {
		return err
	}

	return m.publish(ctx, id, prepared, nil)
}

// Disable removes an installed plugin from the active registry while retaining its package.
func (m *Manager) Disable(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.required[id] {
		return errors.New("required system plugin cannot be disabled or uninstalled")
	}

	item, err := m.find(id)
	if err != nil {
		return err
	}
	if !item.metadata.Enabled {
		return nil
	}

	item.instance = nil
	item.metadata.Enabled = false

	return m.publish(ctx, id, item, nil)
}

// find returns one installed plugin while enforcing manager lifecycle state.
func (m *Manager) find(id string) (managedPlugin, error) {
	if m.closed {
		return managedPlugin{}, errors.New("plugin manager is closed")
	}

	item, ok := m.loaded[id]
	if !ok {
		return managedPlugin{}, fmt.Errorf("plugin %s is not installed", id)
	}

	return item, nil
}

// Upgrade preserves enabled state. New bytes are validated before changing durable state or the registry; in-flight snapshots retain the old version.
func (m *Manager) Upgrade(ctx context.Context, id string, archive []byte) (LoadedPlugin, error) {
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		return LoadedPlugin{}, err
	}
	if pkg.Manifest().ID != id {
		return LoadedPlugin{}, errors.New("upgrade plugin ID does not match")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	previous, err := m.find(id)
	if err != nil {
		return LoadedPlugin{}, err
	}

	item, err := m.prepare(ctx, pkg, previous.metadata.Enabled)
	if err != nil {
		return LoadedPlugin{}, err
	}
	if err = m.publish(ctx, id, item, archive); err != nil {
		return LoadedPlugin{}, err
	}

	return m.describe(item.metadata), nil
}

// Uninstall deletes installed state and retains namespaced settings and data.
// A builtin distribution will seed a missing installation on the next startup;
// disable is the persistent opt-out for plugins shipped by Kumbuka.
func (m *Manager) Uninstall(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.required[id] {
		return errors.New("required system plugin cannot be disabled or uninstalled")
	}
	item, err := m.find(id)
	if err != nil {
		return err
	}
	catalog := make(map[string]managedPlugin, len(m.loaded))
	for key, value := range m.loaded {
		if key != id {
			catalog[key] = value
		}
	}
	order, err := dependencyOrder(catalog)
	if err != nil {
		return err
	}
	commit := func() error { return m.store.DeletePlugin(ctx, id) }
	var old *lifetime
	if item.instance != nil {
		old, err = m.registry.transition(id, nil, true, commit)
	} else {
		err = commit()
	}
	if err != nil {
		return err
	}
	delete(m.loaded, id)
	m.order = order
	if item.instance != nil {
		m.retire(old, item.instance)
	}
	return nil
}
