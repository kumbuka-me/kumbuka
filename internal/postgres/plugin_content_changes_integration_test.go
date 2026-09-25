package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginContentChangeQueue(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	changes := []domain.PluginContentChange{
		{
			PluginID:         "me.kumbuka.tasks",
			Page:             domain.Page{Slug: "guide", Title: "Guide", Markdown: "new"},
			PreviousMarkdown: "old",
			Markdown:         "new",
			ActorID:          7,
		},
		{
			PluginID:         "me.kumbuka.audit",
			Page:             domain.Page{Slug: "guide", Title: "Guide", Markdown: "new"},
			PreviousMarkdown: "old",
			Markdown:         "new",
			ActorID:          7,
		},
	}
	require.NoError(t, database.EnqueuePluginContentChanges(ctx, changes))

	first, err := database.ClaimPluginContentChanges(ctx, 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "me.kumbuka.tasks", first[0].PluginID)
	assert.Equal(t, "guide", first[0].Page.Slug)
	assert.Empty(t, first[0].Page.Markdown)
	assert.Equal(t, "old", first[0].PreviousMarkdown)
	assert.Equal(t, "new", first[0].Markdown)
	assert.Equal(t, 1, first[0].Attempts)

	require.NoError(t, database.RetryPluginContentChange(ctx, first[0].ID, time.Now().Add(time.Hour), "temporary failure"))
	second, err := database.ClaimPluginContentChanges(ctx, 8, time.Minute)
	require.NoError(t, err)
	require.Len(t, second, 1)
	assert.Equal(t, "me.kumbuka.audit", second[0].PluginID)
	require.NoError(t, database.CompletePluginContentChange(ctx, second[0].ID))

	pending, err := database.ClaimPluginContentChanges(ctx, 8, time.Minute)
	require.NoError(t, err)
	assert.Empty(t, pending)

	require.NoError(t, database.RetryPluginContentChange(ctx, first[0].ID, time.Now().Add(-time.Second), "retry now"))
	retried, err := database.ClaimPluginContentChanges(ctx, 8, time.Minute)
	require.NoError(t, err)
	require.Len(t, retried, 1)
	assert.Equal(t, first[0].ID, retried[0].ID)
	assert.Equal(t, 2, retried[0].Attempts)
	require.NoError(t, database.CompletePluginContentChange(ctx, retried[0].ID))
}
