package postgres

import (
	"context"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// PageReviewComments returns line-anchored comments and suggestions for one review request.
func (s *Store) PageReviewComments(ctx context.Context, requestID int64) ([]domain.PageReviewComment, error) {
	rows, err := s.pool.Query(ctx, `
SELECT c.id,c.request_id,coalesce(c.user_id,0),coalesce(u.display_name,u.username,'Deleted user'),
       c.side,c.start_line,c.end_line,c.body,c.is_suggestion,c.original_text,c.replacement_text,
       coalesce(c.applied_by,0),coalesce(applier.display_name,applier.username,''),c.applied_at,c.created_at
FROM page_review_comments c
LEFT JOIN users u ON u.id=c.user_id
LEFT JOIN users applier ON applier.id=c.applied_by
WHERE c.request_id=$1
ORDER BY c.start_line,c.created_at,c.id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	comments := make([]domain.PageReviewComment, 0)
	for rows.Next() {
		var comment domain.PageReviewComment
		if err := rows.Scan(
			&comment.ID,
			&comment.ReviewRequestID,
			&comment.AuthorID,
			&comment.Author,
			&comment.Side,
			&comment.StartLine,
			&comment.EndLine,
			&comment.Body,
			&comment.IsSuggestion,
			&comment.Original,
			&comment.Replacement,
			&comment.AppliedBy,
			&comment.AppliedByName,
			&comment.AppliedAt,
			&comment.CreatedAt,
		); err != nil {
			return nil, err
		}
		comments = append(comments, comment)
	}

	return comments, rows.Err()
}

// AddPageReviewComment persists one validated line comment or suggestion on a locked pending review.
func (s *Store) AddPageReviewComment(
	ctx context.Context,
	requestID int64,
	slug string,
	userID int64,
	side string,
	startLine, endLine int,
	body string,
	suggestion bool,
	original, replacement string,
) (domain.PageReviewComment, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PageReviewComment{}, mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	_, currentRevision, err := lockReviewPage(ctx, tx, requestID, slug)
	if err != nil {
		return domain.PageReviewComment{}, err
	}

	var status string
	var requestedRevision int
	if err := tx.QueryRow(ctx, `
SELECT status,revision_number
FROM page_review_requests
WHERE id=$1
FOR UPDATE`, requestID).Scan(&status, &requestedRevision); err != nil {
		return domain.PageReviewComment{}, mutationError(err)
	}
	if status != domain.PageReviewStatusPending {
		return domain.PageReviewComment{}, domain.ErrReviewClosed
	}
	if requestedRevision != currentRevision {
		return domain.PageReviewComment{}, domain.ErrStaleReview
	}

	var comment domain.PageReviewComment
	err = tx.QueryRow(ctx, `
WITH inserted AS (
  INSERT INTO page_review_comments(
    request_id,user_id,side,start_line,end_line,body,is_suggestion,original_text,replacement_text
  )
  VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
  RETURNING id,request_id,user_id,side,start_line,end_line,body,is_suggestion,
            original_text,replacement_text,applied_by,applied_at,created_at
)
SELECT i.id,i.request_id,coalesce(i.user_id,0),coalesce(u.display_name,u.username,'Deleted user'),
       i.side,i.start_line,i.end_line,i.body,i.is_suggestion,i.original_text,i.replacement_text,
       coalesce(i.applied_by,0),'',i.applied_at,i.created_at
FROM inserted i
LEFT JOIN users u ON u.id=i.user_id`,
		requestID,
		userID,
		side,
		startLine,
		endLine,
		body,
		suggestion,
		original,
		replacement,
	).Scan(
		&comment.ID,
		&comment.ReviewRequestID,
		&comment.AuthorID,
		&comment.Author,
		&comment.Side,
		&comment.StartLine,
		&comment.EndLine,
		&comment.Body,
		&comment.IsSuggestion,
		&comment.Original,
		&comment.Replacement,
		&comment.AppliedBy,
		&comment.AppliedByName,
		&comment.AppliedAt,
		&comment.CreatedAt,
	)
	if err != nil {
		return domain.PageReviewComment{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PageReviewComment{}, mutationError(err)
	}

	return comment, nil
}

// ApplyPageReviewSuggestions atomically creates a new page revision from selected pending suggestions.
func (s *Store) ApplyPageReviewSuggestions(
	ctx context.Context,
	requestID int64,
	slug string,
	expectedRevision int,
	actorID int64,
	suggestionIDs []int64,
	applyAll bool,
	markdown string,
	message string,
	links []string,
	usage *pluginusage.Index,
	render domain.PageRender,
) (domain.Page, error) {
	if len(suggestionIDs) == 0 {
		return domain.Page{}, domain.NewValidationError("suggestion", "Choose at least one review suggestion.")
	}

	pluginUsage, renderedContents, render, err := preparePageDerivedData(usage, render)
	if err != nil {
		return domain.Page{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.Page{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pageID, currentRevision, err := lockReviewPage(ctx, tx, requestID, slug)
	if err != nil {
		return domain.Page{}, err
	}

	var currentMarkdown string
	if err := tx.QueryRow(ctx, `
SELECT markdown_content
FROM pages
WHERE id=$1`, pageID).Scan(&currentMarkdown); err != nil {
		return domain.Page{}, err
	}

	var status string
	var requestedRevision int
	err = tx.QueryRow(ctx, `
SELECT status,revision_number
FROM page_review_requests
WHERE id=$1
FOR UPDATE`, requestID).Scan(&status, &requestedRevision)
	if err != nil {
		return domain.Page{}, err
	}
	if status != domain.PageReviewStatusPending {
		return domain.Page{}, domain.ErrReviewClosed
	}
	if requestedRevision != expectedRevision || currentRevision != expectedRevision {
		return domain.Page{}, domain.ErrStaleReview
	}

	suggestions, err := lockOpenReviewSuggestions(ctx, tx, requestID, suggestionIDs, applyAll)
	if err != nil {
		return domain.Page{}, err
	}
	if !sameReviewSuggestionIDs(suggestionIDs, reviewSuggestionIDs(suggestions)) {
		if applyAll {
			return domain.Page{}, domain.ErrStaleReview
		}
		return domain.Page{}, domain.ErrNotFound
	}

	expectedMarkdown, err := applyLockedReviewSuggestions(currentMarkdown, suggestions)
	if err != nil {
		return domain.Page{}, err
	}
	if markdown != expectedMarkdown {
		return domain.Page{}, domain.ErrStaleReview
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
UPDATE page_review_comments
SET applied_by=$2,applied_at=now(),updated_at=now()
WHERE request_id=$1 AND id=ANY($3)`, requestID, actorID, suggestionIDs); err != nil {
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

// lockOpenReviewSuggestions locks selected or all unapplied suggestions and returns their persisted source edits.
func lockOpenReviewSuggestions(
	ctx context.Context,
	tx pgx.Tx,
	requestID int64,
	suggestionIDs []int64,
	allOpen bool,
) ([]domain.PageReviewComment, error) {
	query := `
SELECT id,side,start_line,end_line,original_text,replacement_text
FROM page_review_comments
WHERE request_id=$1 AND id=ANY($2) AND is_suggestion AND applied_at IS NULL
ORDER BY id
FOR UPDATE`
	arguments := []any{requestID, suggestionIDs}
	if allOpen {
		query = `
SELECT id,side,start_line,end_line,original_text,replacement_text
FROM page_review_comments
WHERE request_id=$1 AND is_suggestion AND applied_at IS NULL
ORDER BY id
FOR UPDATE`
		arguments = []any{requestID}
	}

	rows, err := tx.Query(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	suggestions := make([]domain.PageReviewComment, 0, len(suggestionIDs))
	for rows.Next() {
		var suggestion domain.PageReviewComment
		if err := rows.Scan(
			&suggestion.ID,
			&suggestion.Side,
			&suggestion.StartLine,
			&suggestion.EndLine,
			&suggestion.Original,
			&suggestion.Replacement,
		); err != nil {
			return nil, err
		}
		suggestions = append(suggestions, suggestion)
	}

	return suggestions, rows.Err()
}

// reviewSuggestionIDs returns identifiers from locked review suggestions.
func reviewSuggestionIDs(suggestions []domain.PageReviewComment) []int64 {
	ids := make([]int64, 0, len(suggestions))
	for _, suggestion := range suggestions {
		ids = append(ids, suggestion.ID)
	}

	return ids
}

// applyLockedReviewSuggestions reconstructs the exact source produced by locked persisted suggestions.
func applyLockedReviewSuggestions(markdown string, suggestions []domain.PageReviewComment) (string, error) {
	ordered := slices.Clone(suggestions)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].StartLine != ordered[right].StartLine {
			return ordered[left].StartLine < ordered[right].StartLine
		}
		return ordered[left].EndLine < ordered[right].EndLine
	})

	previousEnd := 0
	for _, suggestion := range ordered {
		if suggestion.Side != domain.PageReviewCommentSideNew || suggestion.StartLine <= previousEnd {
			return "", domain.ErrReviewSuggestionConflict
		}

		original, ok := reviewMarkdownLineRange(markdown, suggestion.StartLine, suggestion.EndLine)
		if !ok || original != suggestion.Original {
			return "", domain.ErrStaleReview
		}
		previousEnd = suggestion.EndLine
	}

	lines := strings.Split(markdown, "\n")
	for index := len(ordered) - 1; index >= 0; index-- {
		suggestion := ordered[index]
		start := suggestion.StartLine - 1
		end := suggestion.EndLine
		replacement := reviewSuggestionLines(suggestion.Replacement)

		updated := make([]string, 0, len(lines)-(end-start)+len(replacement))
		updated = append(updated, lines[:start]...)
		updated = append(updated, replacement...)
		updated = append(updated, lines[end:]...)
		lines = updated
	}

	return strings.Join(lines, "\n"), nil
}

// reviewMarkdownLineRange returns a one-based inclusive source range without trailing separators.
func reviewMarkdownLineRange(markdown string, startLine, endLine int) (string, bool) {
	if startLine <= 0 || endLine < startLine {
		return "", false
	}

	lines := strings.Split(markdown, "\n")
	if startLine > len(lines) || endLine > len(lines) {
		return "", false
	}

	return strings.Join(lines[startLine-1:endLine], "\n"), true
}

// reviewSuggestionLines converts replacement Markdown into lines while treating an empty replacement as deletion.
func reviewSuggestionLines(replacement string) []string {
	if replacement == "" {
		return nil
	}

	return strings.Split(replacement, "\n")
}

// sameReviewSuggestionIDs reports whether two identifier sets contain the same unique values.
func sameReviewSuggestionIDs(expected, actual []int64) bool {
	if len(expected) != len(actual) {
		return false
	}

	seen := make(map[int64]struct{}, len(expected))
	for _, id := range expected {
		if id <= 0 {
			return false
		}
		seen[id] = struct{}{}
	}
	if len(seen) != len(expected) {
		return false
	}

	for _, id := range actual {
		if _, ok := seen[id]; !ok {
			return false
		}
	}

	return true
}
