package store

import (
	"cmp"
	"context"
	"path"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// movedPage describes one page path included in a tree move.
type movedPage struct {
	// id identifies the persisted page row.
	id int64
	// oldSlug is the canonical path before the move.
	oldSlug string
	// newSlug is the canonical path after the move.
	newSlug string
}

// pageSourceEdit contains a Markdown source rewritten after a page move.
type pageSourceEdit struct {
	// id identifies the page whose Markdown changed.
	id int64
	// markdown contains the rewritten canonical Markdown source.
	markdown string
}

// MovePage moves one page, optionally including descendants, and can refactor direct wiki-link targets.
func (s *Store) MovePage(ctx context.Context, oldSlug, newSlug string, options domain.MovePageOptions, user domain.User) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := movePage(ctx, tx, oldSlug, newSlug, options, user); err != nil {
		return mutationError(err)
	}

	return mutationError(tx.Commit(ctx))
}

// BulkMovePages moves every selected page atomically beneath one target path.
func (s *Store) BulkMovePages(ctx context.Context, slugs []string, target string, user domain.User) error {
	target = strings.Trim(strings.TrimSpace(target), "/")
	if target == "" {
		return domain.NewValidationError("target", "A target path is required.")
	}

	ordered := slices.Clone(slugs)
	slices.SortFunc(ordered, func(left, right string) int {
		return cmp.Compare(len(right), len(left))
	})

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	options := domain.MovePageOptions{UpdateIncomingLinks: true, KeepAliases: true}
	for _, slug := range ordered {
		source := strings.Trim(strings.TrimSpace(slug), "/")
		if source == "" {
			return domain.NewValidationError("pages", "Choose valid pages to move.")
		}

		destination := target + "/" + path.Base(source)
		if source == destination {
			return domain.NewValidationError("target", "Choose a different destination for every selected page.")
		}
		if err := movePage(ctx, tx, source, destination, options, user); err != nil {
			return mutationError(err)
		}
	}

	return mutationError(tx.Commit(ctx))
}

// movePage applies one page move inside the caller-owned transaction.
func movePage(ctx context.Context, tx pgx.Tx, oldSlug, newSlug string, options domain.MovePageOptions, user domain.User) error {
	oldSlug, newSlug, err := normalizeMoveSlugs(oldSlug, newSlug, options)
	if err != nil {
		return err
	}

	moved, err := loadMovedPages(ctx, tx, oldSlug, newSlug, options.MoveChildren)
	if err != nil {
		return err
	}
	if len(moved) == 0 {
		return domain.ErrNotFound
	}

	if err := validateMoveDestinations(ctx, tx, moved); err != nil {
		return err
	}
	if err := applyPageMoves(ctx, tx, moved, options.KeepAliases, user.ID); err != nil {
		return err
	}
	if err := retargetMovedPageLinks(ctx, tx, moved); err != nil {
		return err
	}
	if options.UpdateIncomingLinks {
		if err := rewriteIncomingWikiLinks(ctx, tx, moved, user.ID); err != nil {
			return err
		}
	}

	return nil
}

// normalizeMoveSlugs normalizes and validates the source and destination paths for a move.
func normalizeMoveSlugs(oldSlug, newSlug string, options domain.MovePageOptions) (string, string, error) {
	oldSlug = strings.Trim(strings.TrimSpace(oldSlug), "/")
	newSlug = strings.Trim(strings.TrimSpace(newSlug), "/")
	if oldSlug == "" || newSlug == "" || oldSlug == newSlug {
		return "", "", domain.NewValidationError("slug", "Choose a different, non-empty destination path.")
	}
	if options.MoveChildren && strings.HasPrefix(newSlug, oldSlug+"/") {
		return "", "", domain.NewValidationError("slug", "A page tree cannot be moved inside itself.")
	}

	return oldSlug, newSlug, nil
}

// loadMovedPages loads the source page set and calculates each destination path.
func loadMovedPages(ctx context.Context, tx pgx.Tx, oldSlug, newSlug string, moveChildren bool) ([]movedPage, error) {
	query := `
SELECT id,slug
FROM pages
WHERE deleted_at IS NULL AND slug=$1
ORDER BY length(slug),slug`
	if moveChildren {
		query = `
SELECT id,slug
FROM pages
WHERE deleted_at IS NULL AND (slug=$1 OR slug LIKE $1 || '/%')
ORDER BY length(slug),slug`
	}

	rows, err := tx.Query(ctx, query, oldSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var moved []movedPage
	for rows.Next() {
		var item movedPage
		if err := rows.Scan(&item.id, &item.oldSlug); err != nil {
			return nil, err
		}

		item.newSlug = newSlug + strings.TrimPrefix(item.oldSlug, oldSlug)
		moved = append(moved, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return moved, nil
}

// validateMoveDestinations ensures no destination collides with pages or aliases outside the move set.
func validateMoveDestinations(ctx context.Context, tx pgx.Tx, moved []movedPage) error {
	movingIDs := make([]int64, 0, len(moved))
	for _, item := range moved {
		movingIDs = append(movingIDs, item.id)
	}

	for _, item := range moved {
		var conflict bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM pages WHERE slug=$1 AND id<>ALL($2::bigint[])) OR EXISTS(SELECT 1 FROM page_aliases WHERE alias=$1 AND page_id<>ALL($2::bigint[]))`, item.newSlug, movingIDs).Scan(&conflict); err != nil {
			return err
		}
		if conflict {
			return domain.ErrAlreadyExists
		}
	}

	return nil
}

// applyPageMoves updates page paths, navigation icons, and optional aliases deepest-first.
func applyPageMoves(ctx context.Context, tx pgx.Tx, moved []movedPage, keepAliases bool, userID int64) error {
	// Update deepest paths first so unique path constraints never collide with descendants.
	for _, item := range slices.Backward(moved) {
		if _, err := tx.Exec(ctx, `
UPDATE pages
SET slug=$2,updated_by=$3,updated_at=now()
WHERE id=$1`, item.id, item.newSlug, userID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
UPDATE navigation_icons
SET path=$2
WHERE path=$1`, item.oldSlug, item.newSlug); err != nil {
			return err
		}
		if !keepAliases {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO page_aliases(alias,page_id)
VALUES($1,$2)
ON CONFLICT(alias) DO UPDATE
SET page_id=EXCLUDED.page_id`, item.oldSlug, item.id); err != nil {
			return err
		}
	}

	return nil
}

// retargetMovedPageLinks updates normalized link targets that point at moved pages.
func retargetMovedPageLinks(ctx context.Context, tx pgx.Tx, moved []movedPage) error {
	for _, item := range moved {
		if _, err := tx.Exec(ctx, `
UPDATE page_links
SET target_slug=$2
WHERE target_slug=$1`, item.oldSlug, item.newSlug); err != nil {
			return err
		}
	}

	return nil
}

// rewriteIncomingWikiLinks updates direct wiki-link source text and records a revision for each changed page.
func rewriteIncomingWikiLinks(ctx context.Context, tx pgx.Tx, moved []movedPage, userID int64) error {
	rows, err := tx.Query(ctx, `
SELECT id,markdown_content
FROM pages
WHERE deleted_at IS NULL AND markdown_content LIKE '%[[%'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var edits []pageSourceEdit
	for rows.Next() {
		var item pageSourceEdit
		if err := rows.Scan(&item.id, &item.markdown); err != nil {
			return err
		}

		updated := item.markdown
		for _, page := range moved {
			updated = rewriteDirectWikiTarget(updated, page.oldSlug, page.newSlug)
		}
		if updated != item.markdown {
			item.markdown = updated
			edits = append(edits, item)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, edit := range edits {
		if _, err := tx.Exec(ctx, `
UPDATE pages
SET markdown_content=$2,plugin_usage=NULL,updated_by=$3,updated_at=now()
WHERE id=$1`, edit.id, edit.markdown, userID); err != nil {
			return err
		}
		if err := appendPageRevision(ctx, tx, edit.id, edit.markdown, "Update links after page move", userID); err != nil {
			return err
		}
	}

	return nil
}

// rewriteDirectWikiTarget updates direct wiki links while preserving their labels.
func rewriteDirectWikiTarget(source, oldSlug, newSlug string) string {
	source = strings.ReplaceAll(source, "[["+oldSlug+"]]", "[["+newSlug+"]]")
	source = strings.ReplaceAll(source, "[["+oldSlug+"|", "[["+newSlug+"|")
	source = strings.ReplaceAll(source, "[["+oldSlug+"#", "[["+newSlug+"#")

	return source
}
