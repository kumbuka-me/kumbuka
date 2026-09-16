package plugin

import (
	"context"
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/require"
)

type commandWidget struct{}

func (commandWidget) Render(Context, WidgetRequest) (WidgetResult, error) {
	return WidgetResult{}, nil
}

func (commandWidget) Command(_ Context, request WidgetCommandRequest) (sdk.WidgetCommandResult, error) {
	return sdk.WidgetCommandResult{Redirect: "/pages/" + request.Page.Slug}, nil
}

func TestWidgetCommandUsesActiveWidget(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register(Descriptor{ID: "example.widget", Name: "Widget"}, Contributions{
		Widgets: []WidgetModule{{ID: "details", Surface: "page.details", Widget: commandWidget{}}},
	}))
	manager := &Manager{registry: registry}
	page := sdk.Page{Slug: "guide"}
	result, err := manager.WidgetCommand(
		context.Background(),
		"example.widget",
		"details",
		Context{},
		WidgetCommandRequest{Surface: "page.details", Page: &page, Action: "refresh"},
	)
	require.NoError(t, err)
	require.Equal(t, "/pages/guide", result.Redirect)
}

func TestWidgetCommandRejectsUnsafeRedirect(t *testing.T) {
	registry := &Registry{}
	require.NoError(t, registry.Register(Descriptor{ID: "example.widget", Name: "Widget"}, Contributions{
		Widgets: []WidgetModule{{ID: "details", Surface: "page.details", Widget: redirectWidget{"https://example.com"}}},
	}))
	manager := &Manager{registry: registry}
	_, err := manager.WidgetCommand(context.Background(), "example.widget", "details", Context{}, WidgetCommandRequest{Surface: "page.details", Action: "refresh"})
	require.Error(t, err)
}

type redirectWidget struct{ redirect string }

func (redirectWidget) Render(Context, WidgetRequest) (WidgetResult, error) {
	return WidgetResult{}, nil
}
func (w redirectWidget) Command(Context, WidgetCommandRequest) (sdk.WidgetCommandResult, error) {
	return sdk.WidgetCommandResult{Redirect: w.redirect}, nil
}
