package postgres

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedTestPackage(t *testing.T, version string, enabled bool) (plugin.Record, []byte) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{
		"README.md":   "Concurrent seed fixture",
		"plugin.yaml": fmt.Sprintf("api_version: 1\nid: io.concurrent\nname: Concurrent seed\nversion: %s\ndefault_enabled: %t\nmodules:\n  - type: markdown-syntax\n    id: syntax\n    syntax: strikethrough\n", version, enabled),
	} {
		file, err := writer.Create(name)
		require.NoError(t, err)
		_, err = file.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	archive := buffer.Bytes()
	pkg, err := pluginpackage.Read(archive)
	require.NoError(t, err)
	return plugin.Record{ID: pkg.Manifest().ID, Manifest: pkg.Manifest(), Digest: pkg.Digest(), Enabled: enabled, README: pkg.README()}, archive
}

func TestConcurrentPluginSeedsPreserveOneCompleteWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	const count = 12
	records := make([]plugin.Record, count)
	archives := make([][]byte, count)
	for i := range count {
		records[i], archives[i] = seedTestPackage(t, fmt.Sprintf("1.0.%d", i), i%2 == 0)
	}
	gate := make(chan struct{})
	results := make(chan error, count)
	var workers sync.WaitGroup
	for i := range count {
		workers.Add(1)
		go func() { defer workers.Done(); <-gate; results <- database.SeedPlugin(ctx, records[i], archives[i]) }()
	}
	close(gate)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	inventory, err := database.ListPlugins(ctx)
	require.NoError(t, err)
	require.Len(t, inventory, 1)
	winning := inventory[0]
	stored, err := database.PluginPackage(ctx, winning.ID)
	require.NoError(t, err)
	found := false
	for i, record := range records {
		if record.Digest == winning.Digest {
			found = true
			assert.Equal(t, record, winning)
			assert.Equal(t, archives[i], stored)
		}
	}
	require.True(t, found, "metadata, state, and archive must all belong to one candidate")
	// Even a later seed cannot issue an UPDATE against the installed row.
	_, err = database.pool.Exec(ctx, `CREATE FUNCTION reject_seed_update() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'seed updated installation'; END $$;
 CREATE TRIGGER reject_seed_update BEFORE UPDATE ON plugin_installations FOR EACH ROW EXECUTE FUNCTION reject_seed_update()`)
	require.NoError(t, err)
	require.NoError(t, database.SeedPlugin(ctx, records[0], archives[0]))
	inventory, err = database.ListPlugins(ctx)
	require.NoError(t, err)
	assert.Equal(t, winning, inventory[0])
}

type pausedPostgresSeed struct {
	plugin.Store
	reached, resume chan struct{}
}

func (s *pausedPostgresSeed) SeedPlugin(ctx context.Context, r plugin.Record, data []byte) error {
	close(s.reached)
	select {
	case <-s.resume:
		return s.Store.SeedPlugin(ctx, r, data)
	case <-ctx.Done():
		return ctx.Err()
	}
}

type seedRuntime struct{ loads int }

func (r *seedRuntime) Load(context.Context, *pluginpackage.Package) (plugin.Instance, error) {
	r.loads++
	return seedInstance{}, nil
}
func (*seedRuntime) Close(context.Context) error { return nil }

type seedInstance struct{}

func (seedInstance) Contributions() plugin.Contributions { return plugin.Contributions{} }
func (seedInstance) Close(context.Context) error         { return nil }

func TestConcurrentPostgresBootstrapReloadsWinner(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	database, err := Open(ctx, integrationDatabase(t), slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer database.Close()
	_, oldArchive := seedTestPackage(t, "1.0.0", true)
	winning, newArchive := seedTestPackage(t, "2.0.0", false)
	oldDistribution, err := plugin.NewArchiveDistribution([][]byte{oldArchive})
	require.NoError(t, err)
	newDistribution, err := plugin.NewArchiveDistribution([][]byte{newArchive})
	require.NoError(t, err)
	blocked := &pausedPostgresSeed{Store: database, reached: make(chan struct{}), resume: make(chan struct{})}
	oldRuntime, newRuntime := &seedRuntime{}, &seedRuntime{}
	loser := plugin.NewManager(&plugin.Registry{}, oldRuntime, plugin.WithStore(blocked))
	winner := plugin.NewManager(&plugin.Registry{}, newRuntime, plugin.WithStore(database))
	defer loser.Close(context.Background())
	defer winner.Close(context.Background())
	done := make(chan error, 1)
	go func() { done <- loser.Bootstrap(ctx, oldDistribution) }()
	select {
	case <-blocked.reached:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	err = winner.Bootstrap(ctx, newDistribution)
	close(blocked.resume)
	require.NoError(t, err)
	require.NoError(t, <-done)
	for _, manager := range []*plugin.Manager{winner, loser} {
		inventory := manager.Plugins()
		require.Len(t, inventory, 1)
		assert.Equal(t, winning.Manifest, inventory[0].Manifest)
		assert.Equal(t, winning.Digest, inventory[0].Digest)
		assert.False(t, inventory[0].Enabled)
	}
	assert.Zero(t, oldRuntime.loads)
	assert.Zero(t, newRuntime.loads)
	stored, err := database.PluginPackage(ctx, winning.ID)
	require.NoError(t, err)
	assert.Equal(t, newArchive, stored)
}
