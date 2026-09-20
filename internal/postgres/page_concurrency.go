package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// SavePageIfUnchanged persists an existing page only when it has not changed since the editor loaded it.
func (s *Store) SavePageIfUnchanged(
	ctx context.Context,
	expectedUpdatedAt time.Time,
	previousSlug, slug, title, icon, language, markdown, message string,
	tags, links []string,
	groupIDs []int64,
	metadata domain.PageMetadata,
	properties map[string]string,
	render domain.PageRender,
	user domain.User,
) (domain.Page, error) {
	if expectedUpdatedAt.IsZero() || strings.TrimSpace(previousSlug) == "" {
		return domain.Page{}, domain.NewValidationError("expected_updated_at", "Reload the page before saving it.")
	}

	metadata, pluginUsage, renderedContents, render, err := preparePageSave(metadata, render)
	if err != nil {
		return domain.Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := validateAssignableGroup(ctx, tx, metadata.OwnerGroupID, user); err != nil {
		return domain.Page{}, mutationError(err)
	}

	if err := ensurePageUnchanged(ctx, tx, previousSlug, expectedUpdatedAt); err != nil {
		return domain.Page{}, mutationError(err)
	}

	id, err := savePageRecord(ctx, tx, pageSaveRecord{
		previousSlug:     previousSlug,
		slug:             slug,
		title:            title,
		language:         language,
		markdown:         markdown,
		metadata:         metadata,
		render:           render,
		pluginUsage:      pluginUsage,
		renderedContents: renderedContents,
		userID:           user.ID,
	})
	if err != nil {
		return domain.Page{}, mutationError(err)
	}

	if err := savePageIcon(ctx, tx, slug, icon); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := appendPageRevision(ctx, tx, id, markdown, message, user.ID); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := supersedePageReviews(ctx, tx, id); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageTags(ctx, tx, id, tags); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageGroups(ctx, tx, id, groupIDs, user); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageProperties(ctx, tx, id, properties); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageLinks(ctx, tx, id, links); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, mutationError(err)
	}

	return s.GetPage(ctx, slug)
}

// ensurePageUnchanged locks the edited page and compares its persisted version token.
func ensurePageUnchanged(ctx context.Context, tx pgx.Tx, slug string, expectedUpdatedAt time.Time) error {
	var (
		pageID    int64
		deleted   bool
		updatedAt time.Time
	)

	err := tx.QueryRow(ctx, `
SELECT id,deleted_at IS NOT NULL,updated_at
FROM pages
WHERE slug=$1
FOR UPDATE`, strings.TrimSpace(slug)).Scan(&pageID, &deleted, &updatedAt)

	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.ErrNotFound
	case err != nil:
		return err
	case deleted:
		return domain.ErrPageInBin
	case updatedAt.Equal(expectedUpdatedAt):
		return nil
	}

	var currentRevision int
	if err := tx.QueryRow(ctx, `
SELECT coalesce(max(revision_number),0)
FROM page_revisions
WHERE page_id=$1`, pageID).Scan(&currentRevision); err != nil {
		return err
	}

	return &domain.PageEditConflictError{CurrentRevision: currentRevision}
}
