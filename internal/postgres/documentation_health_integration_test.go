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
