package plugincap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type listSource struct{}

func (listSource) Recent(context.Context, int) ([]domain.Page, error) {
	return []domain.Page{{Slug: "recent", Title: "Recent", Icon: "file-text-lucide"}}, nil
}
func (listSource) RecentViewed(context.Context, int) ([]domain.Page, error) {
	return []domain.Page{{Slug: "viewed", Title: "Viewed"}}, nil
}
func (listSource) Favorites(context.Context, int) ([]domain.Page, error) {
	return []domain.Page{{Slug: "favorite", Title: "Favorite"}}, nil
}
func (listSource) Popular(context.Context, int) ([]domain.Page, error) {
	return []domain.Page{{Slug: "popular", Title: "Popular", ViewCount: 7}}, nil
}
func (listSource) RecentEdited(context.Context, int) ([]domain.RecentEdit, error) {
	return []domain.RecentEdit{{Page: domain.Page{Slug: "edited", Title: "Edited"}, RevisionMessage: "Clarify"}}, nil
}

type draftSource struct{}

func (draftSource) Drafts(context.Context, int) ([]domain.PageDraft, error) {
	return []domain.PageDraft{{Key: "page:7", PageID: 7, PageSlug: "guide", Title: "Draft", Stale: true, UpdatedAt: time.Unix(10, 0)}}, nil
}

func TestPageListCapabilitiesExposeImplementedLists(t *testing.T) {
	capabilities := PageListCapabilities(listSource{})
	for _, name := range []string{"pages.recent", "pages.recent-viewed", "pages.favorites", "pages.popular", "pages.recent-edits"} {
		require.Contains(t, capabilities, name)
	}

	value, err := capabilities["pages.recent"](context.Background(), json.RawMessage(`{"Limit":8}`))
	require.NoError(t, err)
	assert.Equal(t, []sdk.Page{{Slug: "recent", Title: "Recent", Icon: "file-text-lucide"}}, value)

	value, err = capabilities["pages.recent-edits"](context.Background(), json.RawMessage(`{"Limit":6}`))
	require.NoError(t, err)
	assert.Equal(t, []sdk.RecentEdit{{Page: sdk.Page{Slug: "edited", Title: "Edited"}, RevisionMessage: "Clarify"}}, value)

	for _, input := range []string{`{"Limit":0}`, `{"Limit":101}`, `{}`} {
		_, err = capabilities["pages.recent"](context.Background(), json.RawMessage(input))
		require.Error(t, err)
	}
}

func TestDraftCapabilitiesHideEditorValues(t *testing.T) {
	capabilities := DraftCapabilities(draftSource{})
	value, err := capabilities["drafts.list"](context.Background(), json.RawMessage(`{"Limit":6}`))
	require.NoError(t, err)
	assert.Equal(t, []sdk.PageDraft{{Key: "page:7", PageID: 7, PageSlug: "guide", Title: "Draft", Stale: true, UpdatedAt: time.Unix(10, 0)}}, value)
}

func TestMergeCapabilitiesCombinesIndependentSets(t *testing.T) {
	first := map[string]plugin.Capability{
		"one": func(context.Context, json.RawMessage) (any, error) { return 1, nil },
	}
	second := map[string]plugin.Capability{
		"two": func(context.Context, json.RawMessage) (any, error) { return 2, nil },
	}
	merged := MergeCapabilities(first, second)
	assert.Len(t, merged, 2)
	assert.Contains(t, merged, "one")
	assert.Contains(t, merged, "two")
}
