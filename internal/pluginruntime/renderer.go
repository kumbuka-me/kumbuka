package pluginruntime

import (
	"context"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/kumbuka/plugins"
)

// Store combines the durable plugin lifecycle and namespaced plugin storage capabilities.
type Store interface {
	plugin.Store
	plugin.Storage
	plugincap.PublicUserSource
}

// NewRenderer constructs the Markdown renderer and its WASM plugin runtime.
func NewRenderer(
	ctx context.Context,
	store Store,
	secretCodec plugin.SecretCodec,
	authorizeHTTP func(context.Context) bool,
	invocationObserver wasm.InvocationObserver,
	logger *slog.Logger,
	version, commit string,
) (*markdown.Renderer, error) {
	archives, err := plugins.Archives()
	if err != nil {
		return nil, err
	}

	renderer, err := markdown.NewWithPluginStore(
		ctx,
		store,
		archives,
		wasm.WithStorage(store),
		wasm.WithUserDirectory(plugincap.PublicUsers{Source: store}),
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
			"users:read",
			"notifications:send",
		),
		wasm.WithLogger(logger),
	)
	if err != nil {
		return nil, err
	}

	renderer.PluginManager().SetSecretCodec(secretCodec)
	renderer.SetArtifactBuild(version, commit)
	return renderer, nil
}
