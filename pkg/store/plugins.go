package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// ReadPluginValue reads plugin value.
func (s *Store) ReadPluginValue(ctx context.Context, id, namespace, key string) ([]byte, bool, error) {
	var value []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3`, id, namespace, key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return value, err == nil, err
}

// ListPluginValues lists plugin values whose keys share prefix in deterministic key order.
func (s *Store) ListPluginValues(ctx context.Context, id, namespace, prefix string) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx, `SELECT key,value FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND left(key,length($3))=$3 ORDER BY key`, id, namespace, prefix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := make(map[string][]byte)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}

// WritePluginValue serializes plugin-scoped writes and enforces a total quota
// across plugin settings and data: 1,024 keys and 16 MiB. Updating a key at the quota
// remains possible. The transaction prevents concurrent quota oversubscription.
func (s *Store) WritePluginValue(ctx context.Context, id, namespace, key string, value []byte) error {
	if value == nil {
		value = []byte{}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 92731))`, id); err != nil {
		return err
	}
	var count, size int64
	err = tx.QueryRow(ctx, `SELECT count(*), COALESCE(sum(octet_length(value)),0) FROM plugin_values WHERE plugin_id=$1 AND NOT (namespace=$2 AND key=$3)`, id, namespace, key).Scan(&count, &size)
	if err != nil {
		return err
	}
	if count >= 1024 || size+int64(len(value)) > 16<<20 {
		return errors.New("plugin storage quota exceeded")
	}
	_, err = tx.Exec(ctx, `INSERT INTO plugin_values(plugin_id,namespace,key,value) VALUES($1,$2,$3,$4) ON CONFLICT(plugin_id,namespace,key) DO UPDATE SET value=EXCLUDED.value`, id, namespace, key, value)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// DeletePluginValue deletes one plugin value. Missing keys are ignored.
func (s *Store) DeletePluginValue(ctx context.Context, id, namespace, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3`, id, namespace, key)
	return err
}
