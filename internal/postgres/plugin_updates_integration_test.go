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

func TestClaimPluginUpdateAnnouncementsDeduplicatesReleases(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	updates := []domain.PluginUpdateNotice{
		{ID: "me.kumbuka.callouts", Name: "Callouts", CurrentVersion: "1.0.0", AvailableVersion: "1.1.0"},
		{ID: "me.kumbuka.tables", Name: "Tables", CurrentVersion: "1.2.0", AvailableVersion: "1.3.0"},
	}

	first, err := database.ClaimPluginUpdateAnnouncements(ctx, updates)
	require.NoError(t, err)
	assert.Equal(t, updates, first)

	second, err := database.ClaimPluginUpdateAnnouncements(ctx, updates)
	require.NoError(t, err)
	assert.Empty(t, second)

	newer := []domain.PluginUpdateNotice{{
		ID:               "me.kumbuka.callouts",
		Name:             "Callouts",
		CurrentVersion:   "1.1.0",
		AvailableVersion: "1.2.0",
	}}
	third, err := database.ClaimPluginUpdateAnnouncements(ctx, newer)
	require.NoError(t, err)
	assert.Equal(t, newer, third)
}

func TestEnabledAdministratorIDsFiltersDisabledAndNonAdministrators(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	t.Cleanup(database.Close)

	var adminID int64
	require.NoError(t, database.pool.QueryRow(ctx, `
INSERT INTO users(username,role,enabled)
VALUES('plugin-update-admin','admin',true)
RETURNING id`).Scan(&adminID))
	_, err = database.pool.Exec(ctx, `
INSERT INTO users(username,role,enabled)
VALUES
  ('plugin-update-disabled-admin','admin',false),
  ('plugin-update-viewer','viewer',true)`)
	require.NoError(t, err)

	ids, err := database.EnabledAdministratorIDs(ctx)
	require.NoError(t, err)
	assert.Contains(t, ids, adminID)
}
