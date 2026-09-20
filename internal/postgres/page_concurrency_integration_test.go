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

func TestSavePageIfUnchangedRejectsStaleEditor(t *testing.T) {
	dsn := integrationDatabase(t)
	ctx := context.Background()
	database, err := Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()

	actor, err := database.EnsureAdministrator(ctx, "concurrency-admin", "", "Concurrency Admin")
	require.NoError(t, err)

	slug := fmt.Sprintf("concurrency/%d", time.Now().UnixNano())
	metadata := domain.PageMetadata{Status: "verified"}
	created, err := database.SavePage(
		ctx,
		"",
		slug,
		"Concurrent page",
		"",
		"",
		"First revision",
		"Created",
		nil,
		nil,
		nil,
		metadata,
		nil,
		domain.PageRender{},
		actor,
	)
	require.NoError(t, err)

	// Ensure the first guarded update receives a distinct updated_at value even on
	// databases whose timestamp precision is lower than Go's time.Time precision.
	time.Sleep(2 * time.Millisecond)

	updated, err := database.SavePageIfUnchanged(
		ctx,
		created.UpdatedAt,
		slug,
		slug,
		"Concurrent page",
		"",
		"",
		"Second revision",
		"First editor",
		nil,
		nil,
		nil,
		metadata,
		nil,
		domain.PageRender{},
		actor,
	)
	require.NoError(t, err)
	assert.Equal(t, "Second revision", updated.Markdown)

	_, err = database.SavePageIfUnchanged(
		ctx,
		created.UpdatedAt,
		slug,
		slug,
		"Concurrent page",
		"",
		"",
		"Stale overwrite",
		"Second editor",
		nil,
		nil,
		nil,
		metadata,
		nil,
		domain.PageRender{},
		actor,
	)
	var conflict *domain.PageEditConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, 2, conflict.CurrentRevision)

	current, err := database.GetPage(ctx, slug)
	require.NoError(t, err)
	assert.Equal(t, "Second revision", current.Markdown)
}
