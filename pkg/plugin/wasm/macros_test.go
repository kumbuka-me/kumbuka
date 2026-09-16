package wasm_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBundledAndInstalledMacrosUsePublicCapabilities verifies bundled and installed macros use public capabilities behavior.
func TestBundledAndInstalledMacrosUsePublicCapabilities(t *testing.T) {
	ctx := context.Background()
	for _, name := range []string{"subpages", "page-report"} {
		data, err := plugins.Packages.ReadFile(name + ".kumbukaplugin")
		require.NoError(t, err)
		var outputs []string
		for _, source := range []plugin.Source{plugin.SourceBundled, plugin.SourceInstalled} {
			runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("pages:read"))
			require.NoError(t, err)
			registry := &plugin.Registry{}
			manager := plugin.NewManager(registry, runtime)
			t.Cleanup(func() { _ = manager.Close(ctx) })
			if source == plugin.SourceBundled {
				require.NoError(t, manager.Bootstrap(ctx, [][]byte{data}))
			} else {
				_, err = manager.Install(ctx, data)
				require.NoError(t, err)
			}
			renderer := markdown.NewWithRegistry(registry)
			capabilities := map[string]plugin.Capability{
				"pages.navigation": func(context.Context, json.RawMessage) (any, error) {
					return []sdk.NavigationNode{{Title: "Child", URL: "/docs/child/", Page: true}}, nil
				},
				"icons.render": func(context.Context, json.RawMessage) (any, error) {
					return `<svg viewBox="0 0 24 24"><path d="M0 0"/></svg><script>bad()</script>`, nil
				},
				"pages.search": func(_ context.Context, data json.RawMessage) (any, error) {
					var query sdk.PageQuery
					require.NoError(t, json.Unmarshal(data, &query))
					assert.Equal(t, "tag:guide", query.Query)
					assert.Equal(t, 20, query.Limit)
					return []sdk.Page{{Slug: "child"}}, nil
				},
				"pages.get": func(_ context.Context, data json.RawMessage) (any, error) {
					var ref sdk.PageRef
					require.NoError(t, json.Unmarshal(data, &ref))
					assert.Equal(t, "child", ref.Slug)
					return sdk.Page{Slug: ref.Slug, Title: "Child <script>bad()</script>", OwnerGroup: "Team", UpdatedAt: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)}, nil
				},
			}
			sourceText := `{{subpages title="Related"}}`
			if name == "page-report" {
				sourceText = `{{pages query="tag:guide"}}`
			}
			got, err := renderer.RenderPageResolvedWithFunctions(sourceText, markdown.Slug, markdown.DefaultOptions(), markdown.Functions{Context: ctx, Capabilities: capabilities})
			require.NoError(t, err)
			assert.Contains(t, got.HTML, "Child")
			assert.NotContains(t, got.HTML, "<script>")
			outputs = append(outputs, got.HTML)
			literal, err := renderer.RenderPageResolvedWithFunctions("```\n"+sourceText+"\n```", markdown.Slug, markdown.DefaultOptions(), markdown.Functions{Capabilities: capabilities})
			require.NoError(t, err)
			assert.NotContains(t, literal.HTML, "Child")
			assert.Contains(t, literal.HTML, "{{")
		}
		assert.Equal(t, outputs[0], outputs[1])
	}
}

// TestPageReportPropagatesAuthorizationFailure verifies page report propagates authorization failure behavior.
func TestPageReportPropagatesAuthorizationFailure(t *testing.T) {
	ctx := context.Background()
	data, err := plugins.Packages.ReadFile("page-report.kumbukaplugin")
	require.NoError(t, err)
	runtime, err := wasm.New(ctx, wasm.Limits{}, wasm.WithPermissions("pages:read"))
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime)
	t.Cleanup(func() { require.NoError(t, manager.Close(context.Background())) })
	require.NoError(t, manager.Bootstrap(ctx, [][]byte{data}))
	renderer := markdown.NewWithRegistry(registry)

	_, err = renderer.RenderPageResolvedWithFunctions(`{{pages query="private"}}`, markdown.Slug, markdown.DefaultOptions(), markdown.Functions{Capabilities: map[string]plugin.Capability{
		"pages.search": func(context.Context, json.RawMessage) (any, error) { return []sdk.Page{{Slug: "private"}}, nil },
		"pages.get":    func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("page unavailable") },
	}})
	require.ErrorContains(t, err, "page unavailable")
}
