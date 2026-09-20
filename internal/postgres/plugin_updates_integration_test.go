package postgres

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotifyPluginUpdatesDeduplicatesAndTargetsEnabledAdministrators(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	var adminID, disabledAdminID, viewerID int64
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,role,enabled)
VALUES('plugin-update-admin','admin',true)
RETURNING id`).Scan(&adminID))
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,role,enabled)
VALUES('plugin-update-disabled-admin','admin',false)
RETURNING id`).Scan(&disabledAdminID))
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,role,enabled)
VALUES('plugin-update-viewer','viewer',true)
RETURNING id`).Scan(&viewerID))

	updates := []domain.PluginUpdateNotice{
		{ID: "me.kumbuka.callouts", Name: "Callouts", CurrentVersion: "1.0.0", AvailableVersion: "1.1.0"},
		{ID: "me.kumbuka.tables", Name: "Tables", CurrentVersion: "1.2.0", AvailableVersion: "1.3.0"},
	}
	require.NoError(t, database.NotifyPluginUpdates(ctx, updates))
	require.NoError(t, database.NotifyPluginUpdates(ctx, updates))

	items, unread, err := database.Notifications(ctx, adminID, 8)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, unread)
	assert.Equal(t, "plugin-update", items[0].Kind)
	assert.Equal(t, "2 plugin updates available", items[0].Title)
	assert.Contains(t, items[0].Body, "Callouts 1.0.0 -> 1.1.0")
	assert.Contains(t, items[0].Body, "Tables 1.2.0 -> 1.3.0")
	assert.Equal(t, "/admin/plugins", items[0].URL)

	items, unread, err = database.Notifications(ctx, disabledAdminID, 8)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, unread)

	items, unread, err = database.Notifications(ctx, viewerID, 8)
	require.NoError(t, err)
	assert.Empty(t, items)
	assert.Zero(t, unread)

	require.NoError(t, database.NotifyPluginUpdates(ctx, []domain.PluginUpdateNotice{{
		ID:               "me.kumbuka.callouts",
		Name:             "Callouts",
		CurrentVersion:   "1.1.0",
		AvailableVersion: "1.2.0",
	}}))
	items, unread, err = database.Notifications(ctx, adminID, 8)
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, 2, unread)
	assert.Equal(t, "Plugin update available", items[0].Title)
	assert.Equal(t, "Callouts 1.2.0 is available; currently 1.1.0.", items[0].Body)
}

func TestNotifyPluginUpdatesWaitsForAnAdministratorBeforeDeduplicating(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	update := []domain.PluginUpdateNotice{{
		ID:               "me.kumbuka.callouts",
		Name:             "Callouts",
		CurrentVersion:   "1.0.0",
		AvailableVersion: "1.1.0",
	}}
	require.NoError(t, database.NotifyPluginUpdates(ctx, update))

	var adminID int64
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,role,enabled)
VALUES('late-plugin-update-admin','admin',true)
RETURNING id`).Scan(&adminID))
	require.NoError(t, database.NotifyPluginUpdates(ctx, update))

	items, unread, err := database.Notifications(ctx, adminID, 8)
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.Equal(t, 1, unread)
}
