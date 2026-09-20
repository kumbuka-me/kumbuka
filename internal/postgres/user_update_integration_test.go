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

func TestAccountUpdateRollsBackWhenCredentialWriteFails(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	var id int64
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username,role,enabled) VALUES('atomic-account','admin',true) RETURNING id`).Scan(&id))
	require.NoError(t, database.SetLocalCredential(ctx, id, "old-hash"))
	require.NoError(t, database.CreateLocalSession(ctx, id, "existing-session", time.Now().Add(time.Hour)))
	group, err := database.CreateGroup(ctx, "Existing")
	require.NoError(t, err)
	require.NoError(t, database.AddGroupMember(ctx, group.ID, id))
	_, err = database.pool.Exec(ctx, `ALTER TABLE local_credentials ADD CONSTRAINT reject_test_hash CHECK (password_hash <> 'rejected-hash')`)
	require.NoError(t, err)
	err = database.UpdateUserAccount(ctx, domain.UserAccountUpdate{UserID: id, Role: "editor", Enabled: false, PasswordHash: "rejected-hash"})
	require.Error(t, err)
	user, err := database.User(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Role)
	assert.True(t, user.Enabled)
	groups, err := database.UserGroups(ctx, id)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, group.ID, groups[0].ID)
	var hash string
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT password_hash FROM local_credentials WHERE user_id=$1`, id).Scan(&hash))
	assert.Equal(t, "old-hash", hash)
	var sessions int
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT count(*) FROM local_sessions WHERE user_id=$1`, id).Scan(&sessions))
	assert.Equal(t, 1, sessions)
	require.NoError(t, database.UpdateUserAccount(ctx, domain.UserAccountUpdate{UserID: id, Role: "editor", Enabled: true, PasswordHash: "new-hash"}))
	user, err = database.User(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, "editor", user.Role)
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT password_hash FROM local_credentials WHERE user_id=$1`, id).Scan(&hash))
	assert.Equal(t, "new-hash", hash)
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT count(*) FROM local_sessions WHERE user_id=$1`, id).Scan(&sessions))
	assert.Zero(t, sessions)
}
