package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/require"
)

func TestOpenNotificationOwnershipAndReadState(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)
	var owner, other, id int64
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('owner') RETURNING id`).Scan(&owner))
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('other') RETURNING id`).Scan(&other))
	require.NoError(t, database.AddNotification(ctx, owner, "mention", "Mention", "Body", "/pages/example#comments"))
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT id FROM notifications WHERE user_id=$1`, owner).Scan(&id))
	_, err = database.OpenNotification(ctx, other, id)
	require.ErrorIs(t, err, domain.ErrNotFound)
	items, unread, err := database.Notifications(ctx, owner, 8)
	require.NoError(t, err)
	require.Equal(t, 1, unread)
	require.Nil(t, items[0].ReadAt)
	destination, err := database.OpenNotification(ctx, owner, id)
	require.NoError(t, err)
	require.Equal(t, "/pages/example#comments", destination)
	var firstRead, timeAfter time.Time
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE id=$1`, id).Scan(&firstRead))
	_, err = database.OpenNotification(ctx, owner, id)
	require.NoError(t, err)
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE id=$1`, id).Scan(&timeAfter))
	require.Equal(t, firstRead, timeAfter)
	_, unread, err = database.Notifications(ctx, owner, 8)
	require.NoError(t, err)
	require.Zero(t, unread)
	_, err = database.OpenNotification(ctx, owner, id+1000)
	require.ErrorIs(t, err, domain.ErrNotFound)
}

func TestMarkNotificationsRead(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	var owner, other int64
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('notification-owner') RETURNING id`).Scan(&owner))
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('notification-other') RETURNING id`).Scan(&other))
	require.NoError(t, database.AddNotification(ctx, owner, "mention", "First", "", "/pages/first"))
	require.NoError(t, database.AddNotification(ctx, owner, "mention", "Second", "", "/pages/second"))

	var firstID int64
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT id FROM notifications WHERE user_id=$1 ORDER BY id LIMIT 1`, owner).Scan(&firstID))

	t.Run("foreign user cannot mark item", func(t *testing.T) {
		require.NoError(t, database.MarkNotificationRead(ctx, other, firstID))

		_, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Equal(t, 2, unread)
	})

	t.Run("marks one owned item", func(t *testing.T) {
		require.NoError(t, database.MarkNotificationRead(ctx, owner, firstID))

		_, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Equal(t, 1, unread)
	})

	t.Run("marks complete inbox", func(t *testing.T) {
		require.NoError(t, database.MarkAllNotificationsRead(ctx, owner))

		_, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Zero(t, unread)
	})
}

func TestNotificationUnreadAndDelete(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	var owner, other int64
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('notification-toggle-owner') RETURNING id`).Scan(&owner))
	require.NoError(t, database.pool.QueryRow(ctx, `INSERT INTO users(username) VALUES('notification-toggle-other') RETURNING id`).Scan(&other))
	require.NoError(t, database.AddNotification(ctx, owner, "mention", "Mutable", "", "/pages/mutable"))

	var id int64
	require.NoError(t, database.pool.QueryRow(ctx, `SELECT id FROM notifications WHERE user_id=$1`, owner).Scan(&id))
	require.NoError(t, database.MarkNotificationRead(ctx, owner, id))

	t.Run("foreign user cannot mark unread", func(t *testing.T) {
		require.NoError(t, database.MarkNotificationUnread(ctx, other, id))

		_, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Zero(t, unread)
	})

	t.Run("owner can mark unread", func(t *testing.T) {
		require.NoError(t, database.MarkNotificationUnread(ctx, owner, id))

		items, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Equal(t, 1, unread)
		require.Nil(t, items[0].ReadAt)
	})

	t.Run("foreign user cannot delete", func(t *testing.T) {
		require.NoError(t, database.DeleteNotification(ctx, other, id))

		items, _, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Len(t, items, 1)
	})

	t.Run("owner can delete", func(t *testing.T) {
		require.NoError(t, database.DeleteNotification(ctx, owner, id))

		items, unread, err := database.Notifications(ctx, owner, 8)
		require.NoError(t, err)
		require.Empty(t, items)
		require.Zero(t, unread)
	})
}
