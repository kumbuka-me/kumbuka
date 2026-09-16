package plugin

import (
	"context"
	"errors"
	"maps"
	"sync"

	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Runtime loads validated packages without depending on their distribution.
type Runtime interface {
	// Load loads the value.
	Load(context.Context, *pluginpackage.Package) (Instance, error)
	// Close releases resources held by the receiver.
	Close(context.Context) error
}

// Instance owns executable contributions and releases their runtime resources.
type Instance interface {
	// Contributions returns the contributions owned by the instance.
	Contributions() Contributions
	// Close releases resources held by the receiver.
	Close(context.Context) error
}

// Source is descriptive metadata only; it never grants a runtime capability.
type Source string

const (
	SourceBundled   Source = "bundled"
	SourceInstalled Source = "installed"
)

// LoadedPlugin describes one plugin known to the manager.
type LoadedPlugin struct {
	// Enabled reports whether the plugin currently contributes active modules.
	Enabled bool
	// Manifest is the validated package manifest for this plugin.
	Manifest pluginpackage.Manifest
	// README is the package documentation rendered in the administration detail modal.
	README string
	// Settings contains persisted boolean settings keyed by settings-module ID.
	Settings map[string]bool
	// Source records whether the package is bundled or installed.
	Source Source
	// Digest stores the content digest used for identity and caching.
	Digest [32]byte
}

// managedPlugin owns the package bytes, metadata, and optional live instance for one plugin.
type managedPlugin struct {
	// archive contains the validated plugin package bytes.
	archive []byte
	// metadata contains externally visible plugin state.
	metadata LoadedPlugin
	// instance owns the active executable plugin instance.
	instance Instance
}

// Manager coordinates package validation, runtime ownership, and atomic registry
// publication. Durable lifecycle state is provided by a small store interface.
type Manager struct {
	// required maps keys to required values used by manager.
	required map[string]bool
	// mu protects concurrent access to the receiver state.
	mu sync.Mutex
	// registry owns the currently published plugin contributions.
	registry *Registry
	// runtime owns executable plugin runtime operations.
	runtime Runtime
	// loaded indexes managed plugins by manifest ID.
	loaded map[string]managedPlugin
	// order preserves deterministic plugin publication order.
	order []string
	// closed prevents lifecycle operations after manager shutdown begins.
	closed bool
	// store provides persistent plugin state storage.
	store Store
	// values provides namespaced persistent settings and data storage.
	values Storage
	// bundled stores embedded package bytes by plugin ID for installed overrides.
	bundled map[string][]byte
	// retirements tracks instances waiting for active render leases to drain.
	retirements []*retirement
}

// NewManager constructs a new manager.
func NewManager(registry *Registry, runtime Runtime, options ...ManagerOption) *Manager {
	m := &Manager{registry: registry, runtime: runtime, loaded: make(map[string]managedPlugin), bundled: make(map[string][]byte), store: &memoryStore{records: make(map[string]Record)}}

	for _, option := range options {
		option(m)
	}

	return m
}

// Plugins returns loaded plugin metadata in stable manager order.
func (m *Manager) Plugins() []LoadedPlugin {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := make([]LoadedPlugin, 0, len(m.loaded))
	seen := make(map[string]bool)

	for _, id := range m.order {
		if loaded, ok := m.loaded[id]; ok && !seen[id] {
			result = append(result, cloneLoaded(loaded.metadata))
			seen[id] = true
		}
	}

	return result
}

// cloneLoaded copies mutable manifest slices before metadata leaves the manager.
func cloneLoaded(metadata LoadedPlugin) LoadedPlugin {
	metadata.Manifest.Modules = append([]pluginpackage.Module(nil), metadata.Manifest.Modules...)
	for i := range metadata.Manifest.Modules {
		metadata.Manifest.Modules[i].Requires = append([]string(nil), metadata.Manifest.Modules[i].Requires...)
		metadata.Manifest.Modules[i].Fields = append([]pluginpackage.ResourceField(nil), metadata.Manifest.Modules[i].Fields...)
	}
	metadata.Manifest.Requires = append([]string(nil), metadata.Manifest.Requires...)
	metadata.Manifest.Permissions = append([]string(nil), metadata.Manifest.Permissions...)
	metadata.Settings = maps.Clone(metadata.Settings)

	return metadata
}

// Close releases resources held by the receiver.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	if !m.closed {
		m.closed = true

		lives := m.registry.detach(m.loaded)
		for id, loaded := range m.loaded {
			if loaded.instance != nil {
				m.retire(lives[id], loaded.instance)
			}
			delete(m.loaded, id)
		}
		m.order = nil
	}
	pending := append([]*retirement(nil), m.retirements...)
	m.mu.Unlock()

	var result error
	for _, retired := range pending {
		select {
		case <-retired.done:
			result = errors.Join(result, retired.err)
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		}
	}

	return errors.Join(result, m.runtime.Close(ctx))
}
