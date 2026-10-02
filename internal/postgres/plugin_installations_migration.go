package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/plugins"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// migratePluginInstallations converts legacy plugin state under the schema transaction while preserving installed archives.
func migratePluginInstallations(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ALTER TABLE plugin_installations
 DROP CONSTRAINT plugin_installations_check,
 ADD COLUMN manifest jsonb, ADD COLUMN digest bytea, ADD COLUMN readme text`); err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `SELECT plugin_id FROM plugin_installations ORDER BY plugin_id`)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		var archive []byte
		if err := tx.QueryRow(ctx, `SELECT package FROM plugin_installations WHERE plugin_id=$1`, id).Scan(&archive); err != nil {
			return err
		}
		if len(archive) == 0 {
			archive, err = (plugins.Distribution{}).Package(ctx, id)
			if err != nil {
				return fmt.Errorf("migrate builtin plugin %s: %w", id, err)
			}
		}
		pkg, err := pluginpackage.Read(archive)
		if err != nil {
			return fmt.Errorf("migrate plugin %s: %w", id, err)
		}
		if pkg.Manifest().ID != id {
			return fmt.Errorf("migrate plugin %s: package identity mismatch", id)
		}
		manifest, err := json.Marshal(pkg.Manifest())
		if err != nil {
			return err
		}
		digest := pkg.Digest()
		if _, err := tx.Exec(ctx, `UPDATE plugin_installations SET manifest=$2,digest=$3,readme=$4,package=$5 WHERE plugin_id=$1`, id, manifest, digest[:], pkg.README(), archive); err != nil {
			return err
		}
	}
	return nil
}
