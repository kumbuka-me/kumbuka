package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// pluginInventorySQL intentionally excludes package; PostgreSQL need not detoast any archive.
const pluginInventorySQL = `SELECT plugin_id,enabled,manifest,digest,readme FROM plugin_installations ORDER BY plugin_id`

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
func (s *Store) PluginPackage(ctx context.Context, id string) ([]byte, error) {
	var archive []byte
	err := s.pool.QueryRow(ctx, `SELECT package FROM plugin_installations WHERE plugin_id=$1`, id).Scan(&archive)
	return archive, err
}
func (s *Store) SavePlugin(ctx context.Context, record plugin.Record, archive []byte) error {
	manifest, err := json.Marshal(record.Manifest)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO plugin_installations(plugin_id,enabled,manifest,digest,readme,package) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(plugin_id) DO UPDATE SET enabled=EXCLUDED.enabled,manifest=EXCLUDED.manifest,digest=EXCLUDED.digest,readme=EXCLUDED.readme,package=EXCLUDED.package`, record.ID, record.Enabled, manifest, record.Digest[:], record.README, archive)
	return err
}
func (s *Store) SetPluginEnabled(ctx context.Context, id string, enabled bool) error {
	result, err := s.pool.Exec(ctx, `UPDATE plugin_installations SET enabled=$2 WHERE plugin_id=$1`, id, enabled)
	if err == nil && result.RowsAffected() != 1 {
		return fmt.Errorf("plugin %s is not installed", id)
	}
	return err
}
func (s *Store) DeletePlugin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_installations WHERE plugin_id=$1`, id)
	return err
}
