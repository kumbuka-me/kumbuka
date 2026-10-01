package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/plugin/wasm"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPluginInstallationSurvivesDatabaseAndRuntimeRestart verifies plugin installation survives database and runtime restart behavior.
func TestPluginInstallationSurvivesDatabaseAndRuntimeRestart(t *testing.T) {
	ctx := context.Background()
	dsn := integrationDatabase(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	database, err := Open(ctx, dsn, logger)
	require.NoError(t, err)
	data, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	runtime, err := wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithInterpreter())
	require.NoError(t, err)
	registry := &plugin.Registry{}
	manager := plugin.NewManager(registry, runtime, plugin.WithStore(database))
	_, err = manager.Install(ctx, data)
	require.NoError(t, err)
	require.NoError(t, manager.Disable(ctx, "me.kumbuka.callouts"))
	require.NoError(t, manager.Close(ctx))
	database.Close()
	database, err = Open(ctx, dsn, logger)
	require.NoError(t, err)
	defer database.Close()
	runtime, err = wasm.New(ctx, wasm.Limits{InitTimeout: 30 * time.Second}, wasm.WithInterpreter())
	require.NoError(t, err)
	registry = &plugin.Registry{}
	manager = plugin.NewManager(registry, runtime, plugin.WithStore(database))
	defer func() { require.NoError(t, manager.Close(ctx)) }()
	require.NoError(t, manager.Bootstrap(ctx, nil))
	assert.False(t, manager.Plugins()[0].Enabled)
	renderer := markdown.NewWithRegistry(registry)
	require.NoError(t, manager.Enable(ctx, "me.kumbuka.callouts"))
	html, err := renderer.Render("!!! note\nRestored")
	require.NoError(t, err)
	assert.Contains(t, html, `class="callout note"`)
	require.NoError(t, manager.Uninstall(ctx, "me.kumbuka.callouts"))
	records, err := database.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Empty(t, records)
}

// TestPluginInventoryAndStateDoNotAccessArchive proves inventory can execute with
// a metadata-only target list and disable cannot trigger a package rewrite.
func TestPluginInventoryAndStateDoNotAccessArchive(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	archive, err := plugins.Packages.ReadFile("strikethrough.kumbukaplugin")
	require.NoError(t, err)
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	record := plugin.Record{ID: pkg.Manifest().ID, Manifest: pkg.Manifest(), Digest: pkg.Digest(), README: pkg.README(), Enabled: true}
	require.NoError(t, database.SavePlugin(ctx, record, archive))
	// A column-level trigger detects UPDATE statements that name the package.
	_, err = database.pool.Exec(ctx, `CREATE FUNCTION reject_plugin_package_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'package rewritten'; END $$;
 CREATE TRIGGER reject_package_update BEFORE UPDATE OF package ON plugin_packages FOR EACH ROW EXECUTE FUNCTION reject_plugin_package_update()`)
	require.NoError(t, err)
	require.NoError(t, database.SetPluginEnabled(ctx, record.ID, false))
	// PostgreSQL EXPLAIN exposes exactly the inventory target list.
	var plan string
	rows, err := database.pool.Query(ctx, "EXPLAIN (VERBOSE, FORMAT JSON) "+pluginInventorySQL)
	require.NoError(t, err)
	require.True(t, rows.Next())
	require.NoError(t, rows.Scan(&plan))
	rows.Close()
	assert.NotContains(t, plan, `\"package\"`)
	assert.NotContains(t, pluginInventorySQL, "package")
	records, err := database.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.False(t, records[0].Enabled)
	assert.Equal(t, record.Manifest, records[0].Manifest)
	assert.Equal(t, record.Digest, records[0].Digest)
	stored, err := database.PluginPackage(ctx, record.ID, record.Digest)
	require.NoError(t, err)
	assert.Equal(t, archive, stored)
}

func TestLegacyPluginMetadataMigration(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, integrationDatabase(t))
	require.NoError(t, err)
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	// Only the legacy installation table is needed to exercise the data migration.
	_, err = tx.Exec(ctx, `CREATE TABLE plugin_installations (
 plugin_id text PRIMARY KEY,source text NOT NULL,enabled boolean NOT NULL,package bytea,
 CONSTRAINT plugin_installations_check CHECK ((source='bundled' AND package IS NULL) OR (source='installed' AND package IS NOT NULL)),
 CONSTRAINT plugin_installations_source_check CHECK (source IN ('bundled','installed')))`)
	require.NoError(t, err)
	builtin, err := plugins.Packages.ReadFile("callouts.kumbukaplugin")
	require.NoError(t, err)
	installed, err := plugins.Packages.ReadFile("strikethrough.kumbukaplugin")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO plugin_installations VALUES ('me.kumbuka.callouts','bundled',false,NULL),('me.kumbuka.strikethrough','installed',true,$1)`, installed)
	require.NoError(t, err)
	require.NoError(t, migratePluginInstallations(ctx, tx))
	ddl, err := migrationFiles.ReadFile("migrations/016_installed_plugin_metadata.sql")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(ddl))
	require.NoError(t, err)
	ddl, err = migrationFiles.ReadFile("migrations/017_versioned_plugin_packages.sql")
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(ddl))
	require.NoError(t, err)
	for id, want := range map[string][]byte{"me.kumbuka.callouts": builtin, "me.kumbuka.strikethrough": installed} {
		var got, digest, manifest []byte
		var enabled bool
		require.NoError(t, tx.QueryRow(ctx, `SELECT p.package,i.digest,i.manifest,i.enabled FROM plugin_installations i JOIN plugin_packages p USING(plugin_id,digest) WHERE i.plugin_id=$1`, id).Scan(&got, &digest, &manifest, &enabled))
		assert.Equal(t, want, got)
		sum := sha256.Sum256(want)
		assert.Equal(t, sum[:], digest)
		assert.Equal(t, id == "me.kumbuka.strikethrough", enabled)
		var metadata pluginpackage.Manifest
		require.NoError(t, json.Unmarshal(manifest, &metadata))
		assert.Equal(t, id, metadata.ID)
	}
}

func TestPluginPackageVersionsSurviveReplacementAndUninstall(t *testing.T) {
	ctx := context.Background()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	old, oldArchive := seedTestPackage(t, "1.0.0", true)
	next, nextArchive := seedTestPackage(t, "2.0.0", true)
	require.NoError(t, database.SavePlugin(ctx, old, oldArchive))
	_, err = database.pool.Exec(ctx, `CREATE FUNCTION reject_replacement() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'replacement rejected'; END $$;
 CREATE TRIGGER reject_replacement BEFORE UPDATE ON plugin_installations FOR EACH ROW EXECUTE FUNCTION reject_replacement()`)
	require.NoError(t, err)
	require.Error(t, database.SavePlugin(ctx, next, nextArchive))
	_, err = database.PluginPackage(ctx, next.ID, next.Digest)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	records, err := database.ListPlugins(ctx)
	require.NoError(t, err)
	require.Equal(t, []plugin.Record{old}, records)
	_, err = database.pool.Exec(ctx, `DROP TRIGGER reject_replacement ON plugin_installations`)
	require.NoError(t, err)
	require.NoError(t, database.SavePlugin(ctx, next, nextArchive))
	records, err = database.ListPlugins(ctx)
	require.NoError(t, err)
	require.Equal(t, []plugin.Record{next}, records)
	require.NoError(t, database.DeletePlugin(ctx, next.ID))
	for _, candidate := range []struct {
		record  plugin.Record
		archive []byte
	}{{old, oldArchive}, {next, nextArchive}} {
		got, err := database.PluginPackage(ctx, candidate.record.ID, candidate.record.Digest)
		require.NoError(t, err)
		assert.Equal(t, candidate.archive, got)
	}
}
