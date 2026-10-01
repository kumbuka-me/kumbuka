package postgres

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// validatePageDestination rejects current slugs or aliases owned by another page.
func validatePageDestination(ctx context.Context, tx pgx.Tx, pageID int64, slug string) error {
	var conflict bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM pages WHERE slug=$1 AND id<>$2) OR EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1 AND page_id<>$2)`, slug, pageID).Scan(&conflict); err != nil {
		return err
	}
	if conflict {
		return domain.ErrAlreadyExists
	}
	return nil
}

// removeCurrentSlugAlias removes a historical alias when its owning page makes that slug current again.
func removeCurrentSlugAlias(ctx context.Context, tx pgx.Tx, pageID int64, slug string) error {
	_, err := tx.Exec(ctx, `
DELETE FROM page_aliases
WHERE alias=$1 AND page_id=$2`, slug, pageID)
	return err
}
