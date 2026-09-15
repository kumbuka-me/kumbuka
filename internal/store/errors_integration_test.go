package store

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSavedSearchUpdateConflictContract(t *testing.T) {
	dsn := integrationDatabase(t)
	ctx := context.Background()
	database, err := Open(ctx, dsn, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	var userID int64
	err = database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('contract-test') RETURNING id`).Scan(&userID)
	require.NoError(t, err)
	require.NoError(t, database.SaveSavedSearch(ctx, userID, 0, "First", "query", false))
	require.NoError(t, database.SaveSavedSearch(ctx, userID, 0, "Second", "query", false))
	searches, err := database.SavedSearches(ctx, userID)
	require.NoError(t, err)
	var secondID int64
	for _, search := range searches {
		if search.Name == "Second" {
			secondID = search.ID
		}
	}
	require.NotZero(t, secondID)
	err = database.SaveSavedSearch(ctx, userID, secondID, "FIRST", "query", false)
	require.ErrorIs(t, err, domain.ErrAlreadyExists)
	cause, ok := errors.AsType[*pgconn.PgError](err)
	require.True(t, ok)
	assert.Equal(t, "23505", cause.Code)
}
