package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageAccessBatchMatchesSingleResource(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	user, err := database.EnsureAdministrator(ctx, "batch-access", "", "Batch Access")
	require.NoError(t, err)
	members, err := database.CreateGroup(ctx, "members")
	require.NoError(t, err)
	outsiders, err := database.CreateGroup(ctx, "outsiders")
	require.NoError(t, err)
	_, err = database.pool.Exec(ctx, `INSERT INTO user_groups(user_id,group_id) VALUES($1,$2)`, user.ID, members.ID)
	require.NoError(t, err)
	require.NoError(t, database.SavePageAccessRule(ctx, "private", members.ID, "edit"))
	require.NoError(t, database.SavePageAccessRule(ctx, "private/closed", outsiders.ID, "view"))
	require.NoError(t, database.SavePageAccessRule(ctx, "private/read", members.ID, "view"))
	paths := []string{"open", "private", "private/child", "private/closed", "private/closed/child", "private/read", "private/read/child", "private-other", "private"}
	batch, err := database.PageAccessBatch(ctx, paths, user.ID)
	require.NoError(t, err)
	for _, path := range paths {
		single, err := database.PageAccess(ctx, path, user.ID)
		require.NoError(t, err)
		assert.Equal(t, single, batch[path], path)
	}
	assert.False(t, batch["private/closed/child"].CanView)
	assert.True(t, batch["private/child"].CanEdit)
	assert.True(t, batch["private/read/child"].CanView)
	assert.False(t, batch["private/read/child"].CanEdit)
	assert.False(t, batch["private-other"].Restricted)
}
