package store

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanMigrationsOrdersNumericVersions(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"010_ten.sql": {Data: []byte("SELECT 10")},
		"002_two.sql": {Data: []byte("SELECT 2")},
		"001_one.sql": {Data: []byte("SELECT 1")},
		"README.md":   {Data: []byte("ignored")},
	})

	planned, err := planMigrations(entries)

	require.NoError(t, err)
	require.Len(t, planned, 3)
	assert.Equal(t, []int{1, 2, 10}, []int{planned[0].version, planned[1].version, planned[2].version})
}

func TestPlanMigrationsRejectsDuplicateVersions(t *testing.T) {
	t.Parallel()

	entries := migrationTestEntries(t, fstest.MapFS{
		"001_create.sql": {Data: []byte("SELECT 1")},
		"001_update.sql": {Data: []byte("SELECT 2")},
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
		"next_schema.sql": {Data: []byte("SELECT 1")},
	})

	_, err := planMigrations(entries)

	require.Error(t, err)
	assert.ErrorContains(t, err, `invalid migration "next_schema.sql"`)
}

// migrationTestEntries returns directory entries for an in-memory migration fixture.
func migrationTestEntries(t *testing.T, files fstest.MapFS) []fs.DirEntry {
	t.Helper()

	entries, err := fs.ReadDir(files, ".")
	require.NoError(t, err)

	return entries
}
