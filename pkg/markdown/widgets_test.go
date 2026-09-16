package markdown

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testWidget struct{}

func (testWidget) Render(_ plugin.Context, request plugin.WidgetRequest) (plugin.WidgetResult, error) {
	revisionURL := "/revisions/" + request.Page.Slug
	return plugin.WidgetResult{
		HTML:    `<h2>Widget</h2><script>alert(1)</script><a href="/pages/ok">Safe</a>`,
		Actions: []sdk.WidgetAction{{ID: "all", Kind: "dialog", Label: "All", URL: revisionURL}},
	}, nil
}

func TestRenderWidgetsUsesSurfaceAndCentralSanitizer(t *testing.T) {
	t.Parallel()

	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example.widget", Name: "Widget"}, plugin.Contributions{
		Widgets: []plugin.WidgetModule{
			{ID: "later", Surface: "page.details", Width: "wide", Order: 20, Widget: testWidget{}},
			{ID: "details", Surface: "page.details", Order: 10, Widget: testWidget{}},
		},
	}))
	renderer := NewWithRegistry(registry)
	page := sdk.Page{Slug: "guide"}

	widgets, err := renderer.RenderWidgets(context.Background(), "page.details", &page, nil, nil, nil)

	require.NoError(t, err)
	require.Len(t, widgets, 2)
	assert.Equal(t, "io.example.widget", widgets[0].PluginID)
	assert.Equal(t, "details", widgets[0].ModuleID)
	assert.Equal(t, "", widgets[0].Width)
	assert.Equal(t, "later", widgets[1].ModuleID)
	assert.Equal(t, "wide", widgets[1].Width)
	assert.Contains(t, widgets[0].HTML, "<h2>Widget</h2>")
	assert.Contains(t, widgets[0].HTML, `href="/pages/ok"`)
	assert.NotContains(t, widgets[0].HTML, "<script")
	require.Len(t, widgets[0].Actions, 1)
	assert.Equal(t, "/revisions/guide", widgets[0].Actions[0].URL)
}

func TestRenderWidgetsRejectsUnknownSurface(t *testing.T) {
	t.Parallel()

	renderer := NewWithRegistry(&plugin.Registry{})
	_, err := renderer.RenderWidgets(context.Background(), "unknown", nil, nil, nil, nil)
	require.Error(t, err)
}

func TestRenderWidgetsSkipsHiddenPluginWidgets(t *testing.T) {
	t.Parallel()

	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(plugin.Descriptor{ID: "io.example.widget", Name: "Widget"}, plugin.Contributions{
		Widgets: []plugin.WidgetModule{
			{ID: "first", Surface: "page.details", Order: 10, Widget: testWidget{}},
			{ID: "second", Surface: "page.details", Order: 20, Widget: testWidget{}},
		},
	}))
	renderer := NewWithRegistry(registry)
	page := sdk.Page{Slug: "guide"}

	widgets, err := renderer.RenderWidgets(
		context.Background(),
		"page.details",
		&page,
		nil,
		nil,
		[]string{plugin.WidgetKey("io.example.widget", "first")},
	)

	require.NoError(t, err)
	require.Len(t, widgets, 1)
	assert.Equal(t, "second", widgets[0].ModuleID)
}
