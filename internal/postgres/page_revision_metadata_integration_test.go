package postgres

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPageRevisionMetadataAndAudit exercises atomic revision snapshots and no-op saves against PostgreSQL.
func TestPageRevisionMetadataAndAudit(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()

	actor, err := database.EnsureAdministrator(ctx, "revision-meta-admin", "", "Revision Metadata Admin")
	require.NoError(t, err)

	slug := fmt.Sprintf("revision-metadata/%d", time.Now().UnixNano())
	options := domain.PageMetadata{Status: domain.PageStatusVerified}
	created, err := database.SavePage(ctx, "", slug, "Original title", "", "", "# Same content\n", "", []string{"original"}, nil, nil, options, map[string]string{"department": "A"}, domain.PageRender{}, actor)
	require.NoError(t, err)
	latest, count, err := database.LatestRevision(ctx, slug)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	require.NotNil(t, latest.Snapshot)
	assert.Equal(t, "Original title", latest.Snapshot.Title)

	unchanged, err := database.SavePageIfUnchanged(ctx, created.UpdatedAt, slug, slug, "Original title", "", "", "# Same content\n", "Irrelevant message", []string{"original"}, nil, nil, options, map[string]string{"department": "A"}, domain.PageRender{}, actor)
	require.NoError(t, err)
	assert.Equal(t, created.UpdatedAt, unchanged.UpdatedAt)
	_, count, err = database.LatestRevision(ctx, slug)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	updated, err := database.SavePageIfUnchanged(ctx, unchanged.UpdatedAt, slug, slug, "New title", "", "", "# Same content\n", "", []string{"other"}, nil, nil, options, map[string]string{"department": "B"}, domain.PageRender{}, actor)
	require.NoError(t, err)
	assert.Equal(t, "New title", updated.Title)
	revisions, err := database.Revisions(ctx, slug)
	require.NoError(t, err)
	require.Len(t, revisions, 2)
	assert.Equal(t, []string{"title", "tags", "properties"}, revisions[0].MetadataChanges)
	assert.Equal(t, "New title", revisions[0].Snapshot.Title)
	assert.Equal(t, "Original title", revisions[1].Snapshot.Title)
	assert.Equal(t, revisions[0].Markdown, revisions[1].Markdown)

	events, err := database.AuditEvents(ctx, 500)
	require.NoError(t, err)
	countForPage := 0
	for _, event := range events {
		if event.ObjectKey != slug {
			continue
		}
		countForPage++
		if event.RevisionNumber == 2 {
			assert.Contains(t, event.Detail, "title: Original title → New title")
			assert.NotContains(t, event.Detail, "department")
			assert.NotContains(t, event.Detail, "A → B")
		}
	}
	assert.Equal(t, 2, countForPage)
}
