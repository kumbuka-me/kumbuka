package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// pluginInventorySQL intentionally excludes package; PostgreSQL need not detoast any archive.
const pluginInventorySQL = `SELECT plugin_id,enabled,manifest,digest,readme FROM plugin_installations ORDER BY plugin_id`

// ListPlugins returns installed plugin metadata without loading archive payloads.
func (s *Store) ListPlugins(ctx context.Context) ([]plugin.Record, error) {
	rows, err := s.pool.Query(ctx, pluginInventorySQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []plugin.Record
	for rows.Next() {
		var record plugin.Record
		var manifest, digest []byte
		if err := rows.Scan(&record.ID, &record.Enabled, &manifest, &digest, &record.README); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(manifest, &record.Manifest); err != nil {
			return nil, err
		}
		if len(digest) != len(record.Digest) {
			return nil, fmt.Errorf("invalid plugin digest for %s", record.ID)
		}
		copy(record.Digest[:], digest)
		records = append(records, record)
	}
	return records, rows.Err()
}

// PluginPackage returns the immutable archive matching a plugin ID and digest.
func (s *Store) PluginPackage(ctx context.Context, id string, digest [32]byte) ([]byte, error) {
	var archive []byte
	err := s.pool.QueryRow(ctx, `SELECT package FROM plugin_packages WHERE plugin_id=$1 AND digest=$2`, id, digest[:]).Scan(&archive)
	return archive, err
}

// SeedPlugin inserts a missing installation while rolling back packages from losing concurrent candidates.
func (s *Store) SeedPlugin(ctx context.Context, record plugin.Record, archive []byte) error {
	return s.persistPlugin(ctx, record, archive, false)
}

// SavePlugin atomically persists an immutable package and replaces its installed pointer while retaining older packages.
func (s *Store) SavePlugin(ctx context.Context, record plugin.Record, archive []byte) error {
	return s.persistPlugin(ctx, record, archive, true)
}

// persistPlugin atomically stores an immutable package and optionally replaces its installation pointer.
func (s *Store) persistPlugin(ctx context.Context, record plugin.Record, archive []byte, replace bool) error {
	manifest, err := json.Marshal(record.Manifest)
	if err != nil {
		return err
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `INSERT INTO plugin_packages(plugin_id,digest,package) VALUES($1,$2,$3) ON CONFLICT(plugin_id,digest) DO NOTHING`, record.ID, record.Digest[:], archive); err != nil {
		return err
	}
	query := `INSERT INTO plugin_installations(plugin_id,enabled,manifest,digest,readme) VALUES($1,$2,$3,$4,$5) ON CONFLICT(plugin_id) DO NOTHING`
	if replace {
		query = `INSERT INTO plugin_installations(plugin_id,enabled,manifest,digest,readme) VALUES($1,$2,$3,$4,$5) ON CONFLICT(plugin_id) DO UPDATE SET enabled=EXCLUDED.enabled,manifest=EXCLUDED.manifest,digest=EXCLUDED.digest,readme=EXCLUDED.readme`
	}
	result, err := tx.Exec(ctx, query, record.ID, record.Enabled, manifest, record.Digest[:], record.README)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return nil
	}
	return tx.Commit(ctx)
}

// SetPluginEnabled changes the enabled state of an installed plugin.
func (s *Store) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	result, err := s.pool.Exec(ctx, `UPDATE plugin_installations SET enabled=$2 WHERE plugin_id=$1`, id, enabled)
	if err == nil && result.RowsAffected() != 1 {
		return fmt.Errorf("plugin %s is not installed", id)
	}
	return err
}

// DeletePlugin removes an installed plugin pointer while retaining immutable package data.
func (s *Store) DeletePlugin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_installations WHERE plugin_id=$1`, id)
	return err
}
