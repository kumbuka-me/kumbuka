package plugincap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/domain"
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
