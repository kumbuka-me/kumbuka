package store

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestPluginStoragePersistsAndIsolatesNamespaces verifies plugin storage persists and isolates namespaces behavior.
func TestPluginStoragePersistsAndIsolatesNamespaces(t *testing.T) {
	dsn := integrationDatabase(t)
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := Open(ctx, dsn, logger)
	require.NoError(t, err)
	require.NoError(t, database.WritePluginValue(ctx, "io.one", "data", "key", []byte("one")))
	require.NoError(t, database.WritePluginValue(ctx, "io.two", "data", "key", []byte("two")))
	require.NoError(t, database.WritePluginValue(ctx, "io.one", "settings", "key", []byte("setting")))
	database.Close()
	database, err = Open(ctx, dsn, logger)
	require.NoError(t, err)
	defer database.Close()
	for _, test := range []struct{ id, namespace, want string }{{"io.one", "data", "one"}, {"io.two", "data", "two"}, {"io.one", "settings", "setting"}} {
		value, found, err := database.ReadPluginValue(ctx, test.id, test.namespace, "key")
		require.NoError(t, err)
		require.True(t, found)
		require.Equal(t, test.want, string(value))
	}
	_, found, err := database.ReadPluginValue(ctx, "io.three", "data", "key")
	require.NoError(t, err)
	require.False(t, found)
	require.Error(t, database.WritePluginValue(ctx, "io.one", "core", "key", nil))
	require.Error(t, database.WritePluginValue(ctx, "io.one", "data", "key", make([]byte, 65537)))
}

// TestPluginStorageQuotaAllowsReplacement verifies plugin storage quota allows replacement behavior.
func TestPluginStorageQuotaAllowsReplacement(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	_, err = database.pool.Exec(ctx, `INSERT INTO plugin_values SELECT 'io.full', 'data', n::text, ''::bytea FROM generate_series(1,1024) n`)
	require.NoError(t, err)
	require.ErrorContains(t, database.WritePluginValue(ctx, "io.full", "settings", "new", nil), "quota")
	require.NoError(t, database.WritePluginValue(ctx, "io.full", "data", "1", []byte("replacement")))
	require.NoError(t, database.WritePluginValue(ctx, "io.other", "data", "new", nil))
	_, err = database.pool.Exec(ctx, `INSERT INTO plugin_values SELECT 'io.bytes', 'data', n::text, decode(repeat('00',65536),'hex') FROM generate_series(1,256) n`)
	require.NoError(t, err)
	require.ErrorContains(t, database.WritePluginValue(ctx, "io.bytes", "settings", "new", []byte("overflow")), "quota")
	require.NoError(t, database.WritePluginValue(ctx, "io.bytes", "data", "1", []byte("smaller")))
}

func TestExternalFileSourceNamespace(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	require.NoError(t, database.WritePluginValue(ctx, "core.external-files", "approved-sources", "docs", []byte("encrypted record")))
	value, found, err := database.ReadPluginValue(ctx, "core.external-files", "approved-sources", "docs")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "encrypted record", string(value))
	require.Error(t, database.WritePluginValue(ctx, "io.guest", "approved-sources", "docs", []byte("forged")))
	_, found, err = database.ReadPluginValue(ctx, "core.external-files", "data", "docs")
	require.NoError(t, err)
	require.False(t, found)
	require.NoError(t, database.DeletePluginValue(ctx, "core.external-files", "approved-sources", "docs"))
}
