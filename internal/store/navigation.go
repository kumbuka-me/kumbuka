package store

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

// NavigationItems returns every page and synthetic folder that can appear in navigation.
func (s *Store) NavigationItems(ctx context.Context) ([]domain.NavigationItem, error) {
	rows, err := s.pool.Query(ctx, `
WITH source AS (
  SELECT string_to_array(slug,'/') AS parts
  FROM pages
  WHERE deleted_at IS NULL AND slug <> ''
), paths AS (
  SELECT DISTINCT array_to_string(parts[1:depth],'/') AS path
  FROM source
  CROSS JOIN LATERAL generate_series(1,array_length(parts,1)) AS depth
)
SELECT
  paths.path,
  coalesce(p.title, regexp_replace(paths.path,'^.*/','')),
  coalesce(i.icon,''),
  p.id IS NOT NULL
FROM paths
LEFT JOIN pages p ON p.slug=paths.path AND p.deleted_at IS NULL
LEFT JOIN navigation_icons i ON i.path=paths.path
ORDER BY paths.path`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var items []domain.NavigationItem

	for rows.Next() {
		var item domain.NavigationItem
		if err := rows.Scan(&item.Path, &item.Title, &item.Icon, &item.Page); err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// NavigationIcons returns icons keyed by their complete navigation paths.
func (s *Store) NavigationIcons(ctx context.Context) (map[string]string, error) {
	rows, err := s.pool.Query(ctx, `
SELECT path,icon
FROM navigation_icons
WHERE icon <> ''`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	icons := make(map[string]string)

	for rows.Next() {
		var path, icon string
		if err := rows.Scan(&path, &icon); err != nil {
			return nil, err
		}

		icons[path] = icon
	}

	return icons, rows.Err()
}

// SetNavigationIcon stores or clears the icon assigned to any navigation path.
func (s *Store) SetNavigationIcon(ctx context.Context, path, icon string) error {
	path = strings.Trim(strings.TrimSpace(path), "/")
	icon = strings.TrimSpace(icon)
	if path == "" {
		return domain.ErrNotFound
	}
	if icon == "" {
		_, err := s.pool.Exec(ctx, `
DELETE FROM navigation_icons
WHERE path=$1`, path)
		return err
	}

	_, err := s.pool.Exec(ctx, `
INSERT INTO navigation_icons(path,icon)
VALUES($1,$2)
ON CONFLICT(path) DO UPDATE SET icon=EXCLUDED.icon`, path, icon)

	return err
}
