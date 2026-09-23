package postgres

import (
	"cmp"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"slices"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// migration describes one validated embedded schema migration.
type migration struct {
	// entry identifies the embedded SQL file.
	entry fs.DirEntry
	// version is the unique numeric prefix recorded in schema_migrations.
	version int
}

// migrate applies unapplied embedded SQL migrations in version order.
func (s *Store) migrate(ctx context.Context, logger *slog.Logger) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := initializeMigrationTransaction(ctx, tx); err != nil {
		return err
	}
	applied, err := applyPendingMigrations(ctx, tx, migrations)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	logAppliedMigrations(logger, applied)
	return nil
}

// initializeMigrationTransaction serializes migration runners and ensures migration history exists.
func initializeMigrationTransaction(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(734627198236)`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`)
	return err
}

// applyPendingMigrations applies and records each migration not already present in history.
func applyPendingMigrations(ctx context.Context, tx pgx.Tx, migrations []migration) ([]migration, error) {
	applied := make([]migration, 0, len(migrations))
	for _, item := range migrations {
		exists, err := migrationApplied(ctx, tx, item.version)
		if err != nil {
			return nil, err
		}
		if exists {
			continue
		}
		if err := applyMigration(ctx, tx, item); err != nil {
			return nil, err
		}
		applied = append(applied, item)
	}
	return applied, nil
}

// migrationApplied reports whether one migration version is already committed.
func migrationApplied(ctx context.Context, tx pgx.Tx, version int) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&exists)
	return exists, err
}

// applyMigration executes one embedded migration and records its version atomically.
func applyMigration(ctx context.Context, tx pgx.Tx, item migration) error {
	sql, err := migrationFiles.ReadFile("migrations/" + item.entry.Name())
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("migration %d: %w", item.version, err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO schema_migrations(version)
VALUES($1)`, item.version); err != nil {
		return fmt.Errorf("migration %d: %w", item.version, err)
	}
	return nil
}

// logAppliedMigrations records committed schema changes after the transaction succeeds.
func logAppliedMigrations(logger *slog.Logger, migrations []migration) {
	for _, item := range migrations {
		logger.Info(
			"applied database migration",
			"event", "database_migration_applied",
			"version", item.version,
			"migration", item.entry.Name(),
		)
	}
}

// loadMigrations validates and orders every embedded SQL migration before the database is touched.
func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}

	return planMigrations(entries)
}

// planMigrations parses migration versions, sorts them numerically, and rejects duplicate versions.
func planMigrations(entries []fs.DirEntry) ([]migration, error) {
	planned := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if !isSQLFile(entry) {
			continue
		}

		version, err := migrationVersion(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("invalid migration %q: %w", entry.Name(), err)
		}

		planned = append(planned, migration{entry: entry, version: version})
	}

	slices.SortFunc(planned, func(left, right migration) int {
		if order := cmp.Compare(left.version, right.version); order != 0 {
			return order
		}
		return strings.Compare(left.entry.Name(), right.entry.Name())
	})

	for index := 1; index < len(planned); index++ {
		previous := planned[index-1]
		current := planned[index]
		if previous.version == current.version {
			return nil, fmt.Errorf(
				"duplicate migration version %d: %s and %s",
				current.version,
				previous.entry.Name(),
				current.entry.Name(),
			)
		}
	}

	return planned, nil
}

// migrationVersion parses the numeric prefix of an embedded migration filename.
func migrationVersion(name string) (int, error) {
	prefix, _, _ := strings.Cut(name, "_")
	version, err := strconv.Atoi(prefix)
	if err != nil {
		return 0, err
	}
	if version < 0 {
		return 0, fmt.Errorf("migration version must not be negative")
	}

	return version, nil
}

// isSQLFile reports whether entry is a regular SQL migration file.
func isSQLFile(entry fs.DirEntry) bool {
	return entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".sql")
}
