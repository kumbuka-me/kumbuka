package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// pageCommentScanner is the minimal row boundary shared by query-row and row-iterator comment scans.
type pageCommentScanner interface {
	Scan(...any) error
}

// scanPageComment reads the complete page-comment projection, including optional inline-suggestion state.
func scanPageComment(row pageCommentScanner) (domain.PageComment, error) {
	var item domain.PageComment
	var suggestionRevision, suggestionStart, suggestionEnd int
	var suggestionOriginal, suggestionReplacement string
	var suggestionAppliedBy int64
	var suggestionAppliedByName string
	var suggestionAppliedAt *time.Time

	err := row.Scan(
		&item.ID,
		&item.PageID,
		&item.ParentID,
		&item.ParentAuthor,
		&item.ParentBody,
		&item.Author,
		&item.Anchor,
		&item.Quote,
		&item.Body,
		&suggestionRevision,
		&suggestionStart,
		&suggestionEnd,
		&suggestionOriginal,
		&suggestionReplacement,
		&item.Resolved,
		&item.CreatedAt,
		&suggestionAppliedBy,
		&suggestionAppliedByName,
		&suggestionAppliedAt,
	)
	if err != nil {
		return domain.PageComment{}, err
	}

	if suggestionRevision > 0 {
		item.Suggestion = &domain.PageCommentSuggestion{
			RevisionNumber: suggestionRevision,
			StartByte:      suggestionStart,
			EndByte:        suggestionEnd,
			Original:       suggestionOriginal,
			Replacement:    suggestionReplacement,
			AppliedBy:      suggestionAppliedBy,
			AppliedByName:  suggestionAppliedByName,
			AppliedAt:      suggestionAppliedAt,
		}
	}

	return item, nil
}

// PageComments returns comments for a page, unresolved first.
func (s *Store) PageComments(ctx context.Context, slug string) ([]domain.PageComment, error) {
	rows, err := s.pool.Query(ctx, `
SELECT c.id,c.page_id,coalesce(c.parent_id,0),coalesce(parent_user.display_name,parent_user.username,'Deleted user'),
       coalesce(parent.body,''),coalesce(u.display_name,u.username,'Deleted user'),c.anchor,c.quote,c.body,
       coalesce(c.suggestion_revision,0),coalesce(c.suggestion_start_byte,0),coalesce(c.suggestion_end_byte,0),
       c.suggestion_original,c.suggestion_replacement,c.resolved_at,c.created_at,coalesce(c.suggestion_applied_by,0),
       coalesce(applied_user.display_name,applied_user.username,''),c.suggestion_applied_at
FROM page_comments c
JOIN pages p ON p.id=c.page_id
LEFT JOIN users u ON u.id=c.user_id
LEFT JOIN page_comments parent ON parent.id=c.parent_id
LEFT JOIN users parent_user ON parent_user.id=parent.user_id
LEFT JOIN users applied_user ON applied_user.id=c.suggestion_applied_by
WHERE p.slug=$1 AND p.deleted_at IS NULL
ORDER BY (c.resolved_at IS NOT NULL),c.created_at`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.PageComment
	for rows.Next() {
		item, err := scanPageComment(rows)
		if err != nil {
			return nil, err
		}

		items = append(items, item)
	}

	return items, rows.Err()
}

// PageComment returns one page comment bound to the expected page slug.
func (s *Store) PageComment(ctx context.Context, slug string, id int64) (domain.PageComment, error) {
	item, err := scanPageComment(s.pool.QueryRow(ctx, `
SELECT c.id,c.page_id,coalesce(c.parent_id,0),coalesce(parent_user.display_name,parent_user.username,'Deleted user'),
       coalesce(parent.body,''),coalesce(u.display_name,u.username,'Deleted user'),c.anchor,c.quote,c.body,
       coalesce(c.suggestion_revision,0),coalesce(c.suggestion_start_byte,0),coalesce(c.suggestion_end_byte,0),
       c.suggestion_original,c.suggestion_replacement,c.resolved_at,c.created_at,coalesce(c.suggestion_applied_by,0),
       coalesce(applied_user.display_name,applied_user.username,''),c.suggestion_applied_at
FROM page_comments c
JOIN pages p ON p.id=c.page_id
LEFT JOIN users u ON u.id=c.user_id
LEFT JOIN page_comments parent ON parent.id=c.parent_id
LEFT JOIN users parent_user ON parent_user.id=parent.user_id
LEFT JOIN users applied_user ON applied_user.id=c.suggestion_applied_by
WHERE p.slug=$1 AND p.deleted_at IS NULL AND c.id=$2`, slug, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageComment{}, domain.ErrCommentNotFound
	}
	if err != nil {
		return domain.PageComment{}, err
	}
	return item, nil
}

// AddPageComment adds a regular comment or reply to a page.
func (s *Store) AddPageComment(
	ctx context.Context, slug string, userID, parentID int64, anchor, quote, body string,
) (domain.PageComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.PageComment{}, domain.NewValidationError("body", "A comment is required.")
	}

	var id int64
	err := s.pool.QueryRow(ctx, `
INSERT INTO page_comments(page_id,user_id,parent_id,anchor,quote,body)
SELECT p.id,$2,nullif($3,0),$4,$5,$6
FROM pages p
WHERE p.slug=$1
  AND p.deleted_at IS NULL
  AND ($3=0 OR EXISTS (
    SELECT 1 FROM page_comments parent WHERE parent.id=$3 AND parent.page_id=p.id
  ))
RETURNING id`, slug, userID, parentID, strings.TrimSpace(anchor), strings.TrimSpace(quote), body).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageComment{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PageComment{}, err
	}

	return s.PageComment(ctx, slug, id)
}

// AddPageSuggestion adds an applicable Markdown suggestion when the source revision is still current.
func (s *Store) AddPageSuggestion(
	ctx context.Context,
	slug string,
	userID int64,
	anchor, body string,
	suggestion domain.PageCommentSuggestion,
	expectedMarkdown string,
) (domain.PageComment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PageComment{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pageID int64
	var currentMarkdown string
	if err := tx.QueryRow(ctx, `
SELECT id,markdown_content
FROM pages
WHERE slug=$1 AND deleted_at IS NULL
FOR UPDATE`, slug).Scan(&pageID, &currentMarkdown); errors.Is(err, pgx.ErrNoRows) {
		return domain.PageComment{}, domain.ErrNotFound
	} else if err != nil {
		return domain.PageComment{}, err
	}

	var currentRevision int
	if err := tx.QueryRow(ctx, `
SELECT coalesce(max(revision_number),0)
FROM page_revisions
WHERE page_id=$1`, pageID).Scan(&currentRevision); err != nil {
		return domain.PageComment{}, err
	}
	if currentMarkdown != expectedMarkdown || currentRevision != suggestion.RevisionNumber {
		return domain.PageComment{}, domain.ErrStaleSuggestion
	}

	var id int64
	if err := tx.QueryRow(ctx, `
INSERT INTO page_comments(
  page_id,user_id,anchor,body,suggestion_revision,suggestion_start_byte,suggestion_end_byte,
  suggestion_original,suggestion_replacement
)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
RETURNING id`,
		pageID,
		userID,
		strings.TrimSpace(anchor),
		strings.TrimSpace(body),
		suggestion.RevisionNumber,
		suggestion.StartByte,
		suggestion.EndByte,
		suggestion.Original,
		suggestion.Replacement,
	).Scan(&id); err != nil {
		return domain.PageComment{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PageComment{}, err
	}

	return s.PageComment(ctx, slug, id)
}

// ApplyPageCommentSuggestion applies one still-current inline suggestion as a new page revision.
func (s *Store) ApplyPageCommentSuggestion(
	ctx context.Context,
	slug string,
	commentID, actorID int64,
	markdown, message string,
	links []string,
	usage *pluginusage.Index,
	render domain.PageRender,
) (domain.Page, error) {
	pluginUsage, renderedContents, render, err := preparePageDerivedData(usage, render)
	if err != nil {
		return domain.Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Page{}, mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var pageID int64
	var currentMarkdown string
	if err := tx.QueryRow(ctx, `
SELECT id,markdown_content
FROM pages
WHERE slug=$1 AND deleted_at IS NULL
FOR UPDATE`, slug).Scan(&pageID, &currentMarkdown); errors.Is(err, pgx.ErrNoRows) {
		return domain.Page{}, domain.ErrNotFound
	} else if err != nil {
		return domain.Page{}, mutationError(err)
	}

	var currentRevision int
	if err := tx.QueryRow(ctx, `
SELECT coalesce(max(revision_number),0)
FROM page_revisions
WHERE page_id=$1`, pageID).Scan(&currentRevision); err != nil {
		return domain.Page{}, mutationError(err)
	}

	var suggestion domain.PageCommentSuggestion
	if err := tx.QueryRow(ctx, `
SELECT coalesce(suggestion_revision,0),coalesce(suggestion_start_byte,0),coalesce(suggestion_end_byte,0),
       suggestion_original,suggestion_replacement,suggestion_applied_at
FROM page_comments
WHERE id=$1 AND page_id=$2
FOR UPDATE`, commentID, pageID).Scan(
		&suggestion.RevisionNumber,
		&suggestion.StartByte,
		&suggestion.EndByte,
		&suggestion.Original,
		&suggestion.Replacement,
		&suggestion.AppliedAt,
	); errors.Is(err, pgx.ErrNoRows) {
		return domain.Page{}, domain.ErrCommentNotFound
	} else if err != nil {
		return domain.Page{}, mutationError(err)
	}

	if suggestion.RevisionNumber <= 0 {
		return domain.Page{}, domain.NewValidationError("suggestion", "This comment does not contain an applicable suggestion.")
	}
	if suggestion.AppliedAt != nil {
		return domain.Page{}, domain.NewValidationError("suggestion", "This suggestion has already been applied.")
	}
	if currentRevision != suggestion.RevisionNumber || !suggestionMatchesSource(currentMarkdown, suggestion) {
		return domain.Page{}, domain.ErrStaleSuggestion
	}

	expectedMarkdown := currentMarkdown[:suggestion.StartByte] + suggestion.Replacement + currentMarkdown[suggestion.EndByte:]
	if markdown != expectedMarkdown {
		return domain.Page{}, domain.ErrStaleSuggestion
	}

	if _, err := tx.Exec(ctx, `
UPDATE pages
SET markdown_content=$2,updated_by=$3,updated_at=now(),plugin_usage=$4::jsonb,
    rendered_html=$5,rendered_contents=$6::jsonb,render_fingerprint=$7,
    rendered_at=CASE WHEN $7<>'' THEN now() ELSE NULL END
WHERE id=$1`, pageID, markdown, actorID, pluginUsage, render.HTML, renderedContents, render.Fingerprint); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := appendPageRevision(ctx, tx, pageID, markdown, message, actorID); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := supersedePageReviews(ctx, tx, pageID); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE page_comments
SET suggestion_applied_by=$2,suggestion_applied_at=now(),updated_at=now()
WHERE id=$1`, commentID, actorID); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := replacePageLinks(ctx, tx, pageID, links); err != nil {
		return domain.Page{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.Page{}, mutationError(err)
	}

	return s.GetPage(ctx, slug)
}

// suggestionMatchesSource reports whether a stored inline suggestion still targets its exact source range.
func suggestionMatchesSource(markdown string, suggestion domain.PageCommentSuggestion) bool {
	if suggestion.StartByte < 0 || suggestion.EndByte <= suggestion.StartByte || suggestion.EndByte > len(markdown) {
		return false
	}

	return markdown[suggestion.StartByte:suggestion.EndByte] == suggestion.Original
}

// ResolvePageComment resolves or reopens one page comment bound to the expected page slug.
func (s *Store) ResolvePageComment(ctx context.Context, slug string, id int64, resolved bool) error {
	var value any
	if resolved {
		value = time.Now()
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE page_comments AS c
SET resolved_at=$3,updated_at=now()
FROM pages AS p
WHERE c.id=$1
  AND p.id=c.page_id
  AND p.slug=$2
  AND p.deleted_at IS NULL`, id, strings.TrimSpace(slug), value)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrCommentNotFound
	}

	return err
}
