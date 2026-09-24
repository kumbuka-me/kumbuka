package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// ReadPluginValue reads plugin value.
func (s *Store) ReadPluginValue(ctx context.Context, id string, namespace plugin.StorageNamespace, key string) ([]byte, bool, error) {
	var value []byte
	err := s.pool.QueryRow(ctx, `SELECT value FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3`, id, string(namespace), key).Scan(&value)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	return value, err == nil, err
}

// ListPluginValues lists plugin values whose keys share prefix in deterministic key order.
func (s *Store) ListPluginValues(ctx context.Context, id string, namespace plugin.StorageNamespace, prefix string) (map[string][]byte, error) {
	rows, err := s.pool.Query(ctx, `SELECT key,value FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND left(key,length($3))=$3 ORDER BY key`, id, string(namespace), prefix)
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

// WritePluginValue serializes plugin-scoped writes and enforces a total quota across plugin settings and data: 1,024 keys and 16 MiB. Updating a key at the quota remains possible. The transaction prevents concurrent quota oversubscription.
func (s *Store) WritePluginValue(ctx context.Context, id string, namespace plugin.StorageNamespace, key string, value []byte) error {
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
	err = tx.QueryRow(ctx, `SELECT count(*), COALESCE(sum(octet_length(value)),0) FROM plugin_values WHERE plugin_id=$1 AND NOT (namespace=$2 AND key=$3)`, id, string(namespace), key).Scan(&count, &size)
	if err != nil {
		return err
	}
	if count >= 1024 || size+int64(len(value)) > 16<<20 {
		return errors.New("plugin storage quota exceeded")
	}
	_, err = tx.Exec(ctx, `INSERT INTO plugin_values(plugin_id,namespace,key,value) VALUES($1,$2,$3,$4) ON CONFLICT(plugin_id,namespace,key) DO UPDATE SET value=EXCLUDED.value`, id, string(namespace), key, value)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ReplacePluginValue atomically moves one plugin value to a new key while preserving quota guarantees.
func (s *Store) ReplacePluginValue(ctx context.Context, id string, namespace plugin.StorageNamespace, oldKey, newKey string, value []byte) error {
	if oldKey == newKey {
		return s.WritePluginValue(ctx, id, namespace, newKey, value)
	}
	if value == nil {
		value = []byte{}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockPluginValues(ctx, tx, id); err != nil {
		return err
	}
	if err := validatePluginValueMove(ctx, tx, id, string(namespace), oldKey, newKey, value); err != nil {
		return err
	}
	if err := movePluginValue(ctx, tx, id, string(namespace), oldKey, newKey, value); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// lockPluginValues serializes quota-sensitive writes for one plugin.
func lockPluginValues(ctx context.Context, tx pgx.Tx, pluginID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 92731))`, pluginID)
	return err
}

// validatePluginValueMove validates source existence, target uniqueness, and resulting quota usage.
func validatePluginValueMove(ctx context.Context, tx pgx.Tx, id, namespace, oldKey, newKey string, value []byte) error {
	oldExists, err := pluginValueExists(ctx, tx, id, namespace, oldKey)
	if err != nil {
		return err
	}
	if !oldExists {
		return plugin.ErrPluginValueNotFound
	}
	newExists, err := pluginValueExists(ctx, tx, id, namespace, newKey)
	if err != nil {
		return err
	}
	if newExists {
		return plugin.ErrPluginValueAlreadyExists
	}
	return validatePluginValueQuota(ctx, tx, id, namespace, oldKey, len(value))
}

// pluginValueExists reports whether one namespaced value exists.
func pluginValueExists(ctx context.Context, tx pgx.Tx, id, namespace, key string) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3)`, id, namespace, key).Scan(&exists)
	return exists, err
}

// validatePluginValueQuota checks quota after excluding the value being replaced.
func validatePluginValueQuota(ctx context.Context, tx pgx.Tx, id, namespace, excludedKey string, valueSize int) error {
	var count, size int64
	err := tx.QueryRow(ctx, `SELECT count(*), COALESCE(sum(octet_length(value)),0) FROM plugin_values WHERE plugin_id=$1 AND NOT (namespace=$2 AND key=$3)`, id, namespace, excludedKey).Scan(&count, &size)
	if err != nil {
		return err
	}
	if count >= 1024 || size+int64(valueSize) > 16<<20 {
		return errors.New("plugin storage quota exceeded")
	}
	return nil
}

// movePluginValue deletes the old key and inserts the replacement inside one transaction.
func movePluginValue(ctx context.Context, tx pgx.Tx, id, namespace, oldKey, newKey string, value []byte) error {
	if _, err := tx.Exec(ctx, `DELETE FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3`, id, namespace, oldKey); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO plugin_values(plugin_id,namespace,key,value) VALUES($1,$2,$3,$4)`, id, namespace, newKey, value)
	return err
}

// DeletePluginValue deletes one plugin value. Missing keys are ignored.
func (s *Store) DeletePluginValue(ctx context.Context, id string, namespace plugin.StorageNamespace, key string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM plugin_values WHERE plugin_id=$1 AND namespace=$2 AND key=$3`, id, string(namespace), key)
	return err
}
