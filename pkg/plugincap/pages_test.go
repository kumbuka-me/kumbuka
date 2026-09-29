package plugincap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// catalog groups the state and data associated with catalog.
type catalog struct {
	// gets stores the value associated with gets.
	gets int
	// searches stores the value associated with searches.
	searches int
}

// Search searches the value.
func (c *catalog) Search(context.Context, string, int) ([]domain.Page, error) {
	c.searches++
	return []domain.Page{{Slug: "shared"}, {Slug: "private"}}, nil
}

// GetPage returns page.
func (c *catalog) GetPage(_ context.Context, slug string) (domain.Page, error) {
	c.gets++
	return domain.Page{Slug: slug, Title: "Public title", Markdown: "SECRET BODY"}, nil
}

// TestSharedCapabilitiesRestrictScopeAndFields verifies shared capabilities restrict scope and fields behavior.
func TestSharedCapabilitiesRestrictScopeAndFields(t *testing.T) {
	source := &catalog{}
	capabilities := Capabilities(SharedPages{Source: source, Slug: "shared"}, nil)
	_, err := capabilities["pages.get"](context.Background(), json.RawMessage(`{"Slug":"private"}`))
	require.Error(t, err)
	assert.Zero(t, source.gets)
	value, err := capabilities["pages.get"](context.Background(), json.RawMessage(`{"Slug":"shared"}`))
	require.NoError(t, err)
	data, err := json.Marshal(value)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "SECRET")
	value, err = capabilities["pages.search"](context.Background(), json.RawMessage(`{"Query":"test","Limit":20}`))
	require.NoError(t, err)
	require.Equal(t, []sdk.Page{{Slug: "shared"}}, value)
	for _, input := range []string{`{"Limit":0}`, `{"Limit":101}`, `{"Limit":-1}`} {
		_, err = capabilities["pages.search"](context.Background(), json.RawMessage(input))
		require.Error(t, err)
	}
	assert.Equal(t, 1, source.searches)
}

func TestPageContentUpdateCapabilityRestrictsCurrentPage(t *testing.T) {
	t.Parallel()
	expected := time.Date(2026, time.September, 29, 12, 0, 0, 0, time.UTC)
	called := false
	capability := PageContentUpdateCapability("guide", func(_ context.Context, request sdk.PageContentUpdate) (sdk.PageContent, error) {
		called = true
		return sdk.PageContent{Slug: request.Slug, Markdown: request.Markdown, UpdatedAt: expected}, nil
	})["pages.update-content"]

	data, err := json.Marshal(sdk.PageContentUpdate{
		Slug: "guide", Markdown: "- [x] done", Message: "Toggle checklist item", ExpectedUpdatedAt: expected,
	})
	require.NoError(t, err)
	value, err := capability(context.Background(), data)
	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, sdk.PageContent{Slug: "guide", Markdown: "- [x] done", UpdatedAt: expected}, value)

	data, err = json.Marshal(sdk.PageContentUpdate{
		Slug: "private", Markdown: "changed", Message: "Toggle checklist item", ExpectedUpdatedAt: expected,
	})
	require.NoError(t, err)
	_, err = capability(context.Background(), data)
	require.Error(t, err)
}
