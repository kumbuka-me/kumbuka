package pluginruntime

import (
	"context"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
)

// Store combines the durable plugin lifecycle and namespaced plugin storage capabilities.
type Store interface {
	plugin.Store
	plugin.Storage
}

// NewRenderer constructs the Markdown renderer and its WASM plugin runtime.
func NewRenderer(
	ctx context.Context,
	store Store,
	secretCodec plugin.SecretCodec,
	authorizeHTTP func(context.Context) bool,
	invocationObserver wasm.InvocationObserver,
	logger, setupLogger *slog.Logger,
	version, commit string,
) (*markdown.Renderer, error) {
	archives, err := plugins.Archives()
	if err != nil {
		setupLogger.Error("load bundled plugins", "event", "plugin_packages_load_failed", "error", err)
		return nil, err
	}

	renderer, err := markdown.NewWithPluginStore(
		ctx,
		store,
		archives,
		wasm.WithStorage(store),
		wasm.WithSecretCodec(secretCodec),
		wasm.WithHTTPAuthorizer(authorizeHTTP),
		wasm.WithInvocationObserver(invocationObserver),
		wasm.WithPermissions(
			"network:http",
			"network:private",
			"network:insecure-tls",
			"activity:read",
			"drafts:read",
			"settings:read",
			"settings:write",
			"storage:read",
			"storage:write",
		),
		wasm.WithLogger(logger.With("component", "plugins")),
	)
	if err != nil {
		setupLogger.Error("create markdown renderer", "event", "markdown_renderer_failed", "error", err)
		return nil, err
	}

	renderer.PluginManager().SetSecretCodec(secretCodec)
	renderer.SetArtifactBuild(version, commit)
	return renderer, nil
}
