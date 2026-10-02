package postgres

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadMigrationsUsesProvidedFilesystem(t *testing.T) {
	t.Parallel()

	planned, err := loadMigrations(fstest.MapFS{
		"migrations/010_ten.sql": &fstest.MapFile{Data: []byte("SELECT 10")},
		"migrations/002_two.sql": &fstest.MapFile{Data: []byte("SELECT 2")},
		"migrations/README.md":   &fstest.MapFile{Data: []byte("ignored")},
	})

	require.NoError(t, err)
	require.Len(t, planned, 2)
	assert.Equal(t, []int{2, 10}, []int{planned[0].version, planned[1].version})
}

func TestLoadMigrationsDoesNotUseEmbeddedFallback(t *testing.T) {
	t.Parallel()

	planned, err := loadMigrations(fstest.MapFS{
		"migrations/042_fixture.sql": &fstest.MapFile{Data: []byte("SELECT 42")},
	})

	require.NoError(t, err)
	require.Len(t, planned, 1)
	assert.Equal(t, 42, planned[0].version)
	assert.Equal(t, "042_fixture.sql", planned[0].entry.Name())
}

func TestLoadMigrationsRequiresMigrationDirectory(t *testing.T) {
	t.Parallel()

	_, err := loadMigrations(fstest.MapFS{
		"001_wrong_root.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
	})

	require.Error(t, err)
}

func TestLoadMigrationsRejectsInvalidProvidedFilename(t *testing.T) {
	t.Parallel()

	_, err := loadMigrations(fstest.MapFS{
		"migrations/next_schema.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
	})

	require.Error(t, err)
	assert.ErrorContains(t, err, `invalid migration "next_schema.sql"`)
}

func TestPlanMigrationsOrdersNumericVersions(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"010_ten.sql": &fstest.MapFile{Data: []byte("SELECT 10")},
		"002_two.sql": &fstest.MapFile{Data: []byte("SELECT 2")},
		"001_one.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
		"README.md":   &fstest.MapFile{Data: []byte("ignored")},
	})

	planned, err := planMigrations(entries)

	require.NoError(t, err)
	require.Len(t, planned, 3)
	assert.Equal(t, []int{1, 2, 10}, []int{planned[0].version, planned[1].version, planned[2].version})
}

func TestPlanMigrationsMarksBaselines(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"009_baseline.sql": &fstest.MapFile{Data: []byte("SELECT 9")},
		"010_feature.sql":  &fstest.MapFile{Data: []byte("SELECT 10")},
	})

	planned, err := planMigrations(entries)

	require.NoError(t, err)
	require.Len(t, planned, 2)
	assert.True(t, planned[0].baseline)
	assert.False(t, planned[1].baseline)
}

func TestPlanMigrationsRejectsDuplicateVersions(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"001_create.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
		"001_update.sql": &fstest.MapFile{Data: []byte("SELECT 2")},
	})

	_, err := planMigrations(entries)

	require.Error(t, err)
	assert.ErrorContains(t, err, "duplicate migration version 1")
	assert.ErrorContains(t, err, "001_create.sql")
	assert.ErrorContains(t, err, "001_update.sql")
}

func TestPlanMigrationsRejectsInvalidVersion(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"next_schema.sql": &fstest.MapFile{Data: []byte("SELECT 1")},
	})

	_, err := planMigrations(entries)

	require.Error(t, err)
	assert.ErrorContains(t, err, `invalid migration "next_schema.sql"`)
}

func TestShouldExecuteBaseline(t *testing.T) {
	t.Parallel()

	t.Run("executes on fresh database", func(t *testing.T) {
		t.Parallel()

		execute, err := shouldExecuteBaseline(9, migrationHistory{highest: -1})

		require.NoError(t, err)
		assert.True(t, execute)
	})

	t.Run("adopts after previous migration", func(t *testing.T) {
		t.Parallel()

		execute, err := shouldExecuteBaseline(9, migrationHistory{count: 1, highest: 8})

		require.NoError(t, err)
		assert.False(t, execute)
	})

	t.Run("rejects database older than baseline predecessor", func(t *testing.T) {
		t.Parallel()

		execute, err := shouldExecuteBaseline(9, migrationHistory{count: 1, highest: 7})

		assert.False(t, execute)
		require.Error(t, err)
		assert.ErrorContains(t, err, "highest applied migration 8")
		assert.ErrorContains(t, err, "found 7")
	})

	t.Run("rejects inconsistent newer history", func(t *testing.T) {
		t.Parallel()

		execute, err := shouldExecuteBaseline(9, migrationHistory{count: 1, highest: 10})

		assert.False(t, execute)
		require.Error(t, err)
		assert.ErrorContains(t, err, "highest applied migration 8")
		assert.ErrorContains(t, err, "found 10")
	})

	t.Run("works across three digit boundary", func(t *testing.T) {
		t.Parallel()

		execute, err := shouldExecuteBaseline(100, migrationHistory{count: 1, highest: 99})

		require.NoError(t, err)
		assert.False(t, execute)
	})
}

func TestMigrationHistoryRecord(t *testing.T) {
	t.Parallel()

	history := migrationHistory{count: 2, highest: 8}
	history.record(9)

	assert.Equal(t, int64(3), history.count)
	assert.Equal(t, 9, history.highest)
}

// migrationTestEntries returns directory entries for an in-memory migration fixture.
func migrationTestEntries(t *testing.T, files fstest.MapFS) []fs.DirEntry {
	t.Helper()

	entries, err := fs.ReadDir(files, ".")
	require.NoError(t, err)

	return entries
}
