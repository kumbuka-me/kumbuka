package plugins

import (
	"context"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/require"
	"testing"
)

type catalogRuntime struct{}

func (catalogRuntime) Load(context.Context, *pluginpackage.Package) (plugin.Instance, error) {
	return catalogInstance{}, nil
}
func (catalogRuntime) Close(context.Context) error { return nil }

type catalogInstance struct{}

func (catalogInstance) Contributions() plugin.Contributions { return plugin.Contributions{} }
func (catalogInstance) Close(context.Context) error         { return nil }
func TestEmptyStoreSeedsEveryEmbeddedPackage(t *testing.T) {
	ctx := context.Background()
	m := plugin.NewManager(&plugin.Registry{}, catalogRuntime{})
	require.NoError(t, m.Bootstrap(ctx, Distribution{}))
	defer m.Close(ctx)
	require.Len(t, m.Plugins(), len(catalog))
	for _, item := range m.Plugins() {
		require.Equal(t, item.Manifest.DefaultEnabled, item.Enabled)
		require.NotNil(t, item.Builtin)
		require.Equal(t, item.Builtin.Digest, item.Digest)
	}
}
