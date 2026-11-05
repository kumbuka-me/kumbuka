package postgres

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDocumentationHealthTreatsAliasLinksAsIncomingReferences(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()

	var targetID int64
	err = database.pool.QueryRow(ctx, `
INSERT INTO pages(slug,title)
VALUES('guides/current','Current guide')
RETURNING id`).Scan(&targetID)
	require.NoError(t, err)

	var sourceID int64
	err = database.pool.QueryRow(ctx, `
INSERT INTO pages(slug,title)
VALUES('index','Index')
RETURNING id`).Scan(&sourceID)
	require.NoError(t, err)

	_, err = database.pool.Exec(ctx, `INSERT INTO page_aliases(alias,page_id) VALUES('guides/old',$1)`, targetID)
	require.NoError(t, err)
	_, err = database.pool.Exec(ctx, `INSERT INTO page_links(source_page_id,target_slug) VALUES($1,'guides/old')`, sourceID)
	require.NoError(t, err)

	health, err := database.DocumentationHealth(ctx, time.Now().Add(-365*24*time.Hour))

	require.NoError(t, err)
	assert.False(t, slices.ContainsFunc(health.OrphanPages, func(page domain.Page) bool {
		return page.Slug == "guides/current"
	}))
}

func TestDocumentationHealthIncludesStablePageIDs(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()

	var draftID, deprecatedID int64
	err = database.pool.QueryRow(ctx, `
INSERT INTO pages(slug,title,status,review_interval_days,created_at,updated_at)
VALUES('health/draft','Draft', 'draft', 7, now()-interval '400 days', now()-interval '400 days')
RETURNING id`).Scan(&draftID)
	require.NoError(t, err)
	err = database.pool.QueryRow(ctx, `
INSERT INTO pages(slug,title,status)
VALUES('health/deprecated','Deprecated','deprecated')
RETURNING id`).Scan(&deprecatedID)
	require.NoError(t, err)

	health, err := database.DocumentationHealth(ctx, time.Now().AddDate(0, -6, 0))
	require.NoError(t, err)

	for _, category := range []struct {
		name  string
		pages []domain.Page
		id    int64
		slug  string
	}{
		{name: "orphan", pages: health.OrphanPages, id: draftID, slug: "health/draft"},
		{name: "untagged", pages: health.UntaggedPages, id: draftID, slug: "health/draft"},
		{name: "uniconed", pages: health.UniconedPages, id: draftID, slug: "health/draft"},
		{name: "stale", pages: health.StalePages, id: draftID, slug: "health/draft"},
		{name: "review due", pages: health.ReviewDue, id: draftID, slug: "health/draft"},
		{name: "draft", pages: health.DraftPages, id: draftID, slug: "health/draft"},
		{name: "deprecated", pages: health.Deprecated, id: deprecatedID, slug: "health/deprecated"},
	} {
		t.Run(category.name, func(t *testing.T) {
			assert.True(t, slices.ContainsFunc(category.pages, func(page domain.Page) bool {
				return page.ID == category.id && page.Slug == category.slug
			}), "documentation-health rows must include the stable page ID")
		})
	}
}
