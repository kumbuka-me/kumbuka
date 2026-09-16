package store

import (
	"context"

	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// ListPlugins returns durable plugin installation records in stable ID order.
func (s *Store) ListPlugins(ctx context.Context) ([]plugin.Record, error) {
	rows, err := s.pool.Query(ctx, `SELECT plugin_id,source,enabled,package FROM plugin_installations ORDER BY plugin_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var records []plugin.Record
	for rows.Next() {
		var record plugin.Record
		if err := rows.Scan(&record.ID, &record.Source, &record.Enabled, &record.Package); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

// SavePlugin creates or replaces one durable plugin installation record.
func (s *Store) SavePlugin(ctx context.Context, record plugin.Record) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO plugin_installations(plugin_id,source,enabled,package) VALUES($1,$2,$3,$4) ON CONFLICT(plugin_id) DO UPDATE SET source=EXCLUDED.source,enabled=EXCLUDED.enabled,package=EXCLUDED.package`, record.ID, record.Source, record.Enabled, record.Package)
	return err
}

// DeletePlugin removes one durable plugin installation record.
func (s *Store) DeletePlugin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_installations WHERE plugin_id=$1`, id)
	return err
}
