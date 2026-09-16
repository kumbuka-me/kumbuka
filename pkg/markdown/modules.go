package markdown

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
)

// New constructs a renderer with no distribution packages. Callers that own
// bundled packages can pass them explicitly to NewWithPluginStore or
// NewWithPluginPackages.
func New(ctx context.Context, runtimeOptions ...wasm.Option) (*Renderer, error) {
	return newWithPluginPackages(ctx, nil, nil, nil, runtimeOptions...)
}

// NewWithPluginStore restores durable plugin lifecycle state and bootstraps the
// supplied distribution packages before rendering.
func NewWithPluginStore(ctx context.Context, store plugin.Store, archives [][]byte, runtimeOptions ...wasm.Option) (*Renderer, error) {
	return newWithPluginPackages(ctx, store, archives, nil, runtimeOptions...)
}

// NewWithPluginPackages constructs an isolated renderer from only the supplied
// packages. Required IDs are force-enabled regardless of distribution defaults so
// callers can build a renderer from an explicitly selected package set.
func NewWithPluginPackages(ctx context.Context, archives [][]byte, required []string, runtimeOptions ...wasm.Option) (*Renderer, error) {
	return newWithPluginPackages(ctx, nil, archives, required, runtimeOptions...)
}

func newWithPluginPackages(
	ctx context.Context,
	store plugin.Store,
	archives [][]byte,
	required []string,
	runtimeOptions ...wasm.Option,
) (*Renderer, error) {
	registry := &plugin.Registry{}
	options := []wasm.Option{wasm.WithPermissions("pages:read", "pages:content", "browser:render")}
	options = append(options, runtimeOptions...)
	runtime, err := wasm.New(ctx, wasm.Limits{}, options...)
	if err != nil {
		return nil, err
	}
	managerOptions := make([]plugin.ManagerOption, 0, 2)
	if store != nil {
		managerOptions = append(managerOptions, plugin.WithStore(store))
		if values, ok := store.(plugin.Storage); ok {
			managerOptions = append(managerOptions, plugin.WithStorage(values))
		}
	}
	if len(required) != 0 {
		managerOptions = append(managerOptions, plugin.WithRequiredPlugins(required...))
	}
	manager := plugin.NewManager(registry, runtime, managerOptions...)
	if err := manager.Bootstrap(ctx, archives); err != nil {
		_ = manager.Close(context.Background())
		return nil, err
	}
	return NewWithManager(registry, manager), nil
}

// pluginFeatures returns lifecycle and plugin-owned settings for the current renderer.
func (r *Renderer) pluginFeatures() map[string]bool {
	if r.manager == nil {
		return nil
	}
	return r.manager.FeatureSettings()
}

// PluginManager exposes lifecycle operations to the trusted application layer.
// Renderers built from an external registry have no owned manager.
func (r *Renderer) PluginManager() *plugin.Manager { return r.manager }
