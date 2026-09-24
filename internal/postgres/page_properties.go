package postgres

import (
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageProperties returns structured properties for one page.
func (s *Store) PageProperties(ctx context.Context, pageID int64) ([]domain.PageProperty, error) {
	rows, err := s.importQuery(ctx).Query(ctx, `
SELECT key,value
FROM page_properties
WHERE page_id=$1
ORDER BY lower(key),key`, pageID)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var properties []domain.PageProperty

	for rows.Next() {
		var item domain.PageProperty
		if err := rows.Scan(&item.Key, &item.Value); err != nil {
			return nil, err
		}

		properties = append(properties, item)
	}

	return properties, rows.Err()
}

// replacePageProperties atomically replaces the structured properties for a page.
func replacePageProperties(ctx context.Context, tx pgx.Tx, pageID int64, properties map[string]string) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM page_properties
WHERE page_id=$1`, pageID); err != nil {
		return err
	}

	keys := slices.Sorted(maps.Keys(properties))

	for _, key := range keys {
		value := strings.TrimSpace(properties[key])
		key = strings.TrimSpace(key)
		if key == "" || value == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO page_properties(page_id,key,value)
VALUES($1,$2,$3)`, pageID, key, value); err != nil {
			return err
		}
	}

	return nil
}
