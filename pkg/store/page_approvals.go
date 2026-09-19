package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// PageReviewRequest returns the active review workflow item for a page.
func (s *Store) PageReviewRequest(ctx context.Context, slug string) (domain.PageReviewRequest, error) {
	item, err := scanPageReviewRequest(s.pool.QueryRow(ctx, `
SELECT rr.id,p.slug,rr.revision_number,rr.requested_by,requester.username,
       coalesce(rr.reviewer_group_id,0),coalesce(g.name,''),
       coalesce(rr.reviewed_by,0),coalesce(reviewer.username,''),rr.status,
       rr.note,rr.decision_note,rr.previous_status,rr.created_at,rr.updated_at
FROM page_review_requests rr
JOIN pages p ON p.id=rr.page_id
JOIN users requester ON requester.id=rr.requested_by
LEFT JOIN wiki_groups g ON g.id=rr.reviewer_group_id
LEFT JOIN users reviewer ON reviewer.id=rr.reviewed_by
WHERE p.slug=$1 AND p.deleted_at IS NULL
  AND rr.id=(
    SELECT newest.id
    FROM page_review_requests newest
    WHERE newest.page_id=p.id
    ORDER BY newest.created_at DESC,newest.id DESC
    LIMIT 1
  )
  AND rr.status IN ('pending','changes_requested')`, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageReviewRequest{}, nil
	}
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	item.Reviewers, err = s.pageReviewRequestReviewers(ctx, item.ID)
	return item, err
}

// PageReviewRequestByID returns one review request regardless of completion state.
func (s *Store) PageReviewRequestByID(ctx context.Context, id int64, slug string) (domain.PageReviewRequest, error) {
	item, err := scanPageReviewRequest(s.pool.QueryRow(ctx, `
SELECT rr.id,p.slug,rr.revision_number,rr.requested_by,requester.username,
       coalesce(rr.reviewer_group_id,0),coalesce(g.name,''),
       coalesce(rr.reviewed_by,0),coalesce(reviewer.username,''),rr.status,
       rr.note,rr.decision_note,rr.previous_status,rr.created_at,rr.updated_at
FROM page_review_requests rr
JOIN pages p ON p.id=rr.page_id
JOIN users requester ON requester.id=rr.requested_by
LEFT JOIN wiki_groups g ON g.id=rr.reviewer_group_id
LEFT JOIN users reviewer ON reviewer.id=rr.reviewed_by
WHERE rr.id=$1 AND p.slug=$2 AND p.deleted_at IS NULL`, id, slug))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageReviewRequest{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	item.Reviewers, err = s.pageReviewRequestReviewers(ctx, item.ID)
	return item, err
}

// ReviewUsers resolves enabled editor or administrator accounts by exact username.
func (s *Store) ReviewUsers(ctx context.Context, usernames []string) ([]domain.User, error) {
	if len(usernames) == 0 {
		return nil, nil
	}

	rows, err := s.pool.Query(ctx, `
SELECT id,username,email,display_name,role
FROM users
WHERE lower(username)=ANY($1) AND enabled AND role IN ('admin','editor')
ORDER BY lower(username),id`, usernames)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0, len(usernames))
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// ReviewGroup resolves a collaboration group that can be assigned to a review request.
func (s *Store) ReviewGroup(ctx context.Context, id int64) (domain.Group, error) {
	var group domain.Group
	err := s.pool.QueryRow(ctx, `
SELECT id,name
FROM wiki_groups
WHERE id=$1`, id).Scan(&group.ID, &group.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Group{}, domain.ErrNotFound
	}
	return group, err
}

// ReviewGroups returns only the fields needed by page review target selectors.
// Unlike Groups, this deliberately avoids membership and page-count aggregates.
func (s *Store) ReviewGroups(ctx context.Context) ([]domain.Group, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name
FROM wiki_groups
ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []domain.Group
	for rows.Next() {
		var group domain.Group
		if err := rows.Scan(&group.ID, &group.Name); err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// RequestPageReview opens a review for the current revision and stores explicit targets.
func (s *Store) RequestPageReview(
	ctx context.Context,
	slug string,
	actorID int64,
	reviewerIDs []int64,
	reviewerGroupID int64,
	note string,
) (domain.PageReviewRequest, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pageID, revisionNumber, previousStatus, err := reviewPageState(ctx, tx, slug)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	var id int64
	err = tx.QueryRow(ctx, `
INSERT INTO page_review_requests(
  page_id,revision_number,requested_by,reviewer_group_id,status,note,previous_status
)
VALUES($1,$2,$3,NULLIF($4,0),'pending',$5,$6)
RETURNING id`, pageID, revisionNumber, actorID, reviewerGroupID, note, previousStatus).Scan(&id)
	if err != nil {
		return domain.PageReviewRequest{}, mutationError(err)
	}

	if err := replacePageReviewers(ctx, tx, id, reviewerIDs); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE pages
SET status='draft',updated_at=now()
WHERE id=$1`, pageID); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if err := notifyPageReviewTargets(ctx, tx, id, actorID, "Review requested for ", note); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PageReviewRequest{}, err
	}

	return s.PageReviewRequest(ctx, slug)
}

// UpdatePageReview changes only the targets and request note of a pending review.
func (s *Store) UpdatePageReview(
	ctx context.Context,
	id int64,
	slug string,
	actorID int64,
	reviewerIDs []int64,
	reviewerGroupID int64,
	note string,
) (domain.PageReviewRequest, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	err = tx.QueryRow(ctx, `
SELECT rr.status
FROM page_review_requests rr
JOIN pages p ON p.id=rr.page_id
WHERE rr.id=$1 AND p.slug=$2 AND p.deleted_at IS NULL
FOR UPDATE OF rr`, id, slug).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageReviewRequest{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	if status != domain.PageReviewStatusPending {
		return domain.PageReviewRequest{}, domain.ErrReviewClosed
	}

	if _, err := tx.Exec(ctx, `
UPDATE page_review_requests
SET reviewer_group_id=NULLIF($2,0),note=$3,updated_at=now()
WHERE id=$1`, id, reviewerGroupID, note); err != nil {
		return domain.PageReviewRequest{}, mutationError(err)
	}
	if err := replacePageReviewers(ctx, tx, id, reviewerIDs); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if err := notifyPageReviewTargets(ctx, tx, id, actorID, "Review request updated for ", note); err != nil {
		return domain.PageReviewRequest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.PageReviewRequest{}, err
	}

	return s.PageReviewRequest(ctx, slug)
}

// CancelPageReview closes a pending request and restores the pre-request lifecycle when safe.
func (s *Store) CancelPageReview(ctx context.Context, id int64, slug string, actorID int64) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pageID, currentRevision, err := lockReviewPage(ctx, tx, id, slug)
	if err != nil {
		return "", err
	}

	var status, previousStatus string
	var requestedRevision int
	err = tx.QueryRow(ctx, `
SELECT status,previous_status,revision_number
FROM page_review_requests
WHERE id=$1
FOR UPDATE`, id).Scan(&status, &previousStatus, &requestedRevision)
	if err != nil {
		return "", err
	}
	if status != domain.PageReviewStatusPending {
		return "", domain.ErrReviewClosed
	}

	if _, err := tx.Exec(ctx, `
UPDATE page_review_requests
SET status='canceled',updated_at=now()
WHERE id=$1`, id); err != nil {
		return "", err
	}
	if err := restoreCanceledReviewStatus(ctx, tx, pageID, requestedRevision, currentRevision, previousStatus); err != nil {
		return "", err
	}
	if err := notifyPageReviewTargets(ctx, tx, id, actorID, "Review canceled for ", "The review request was canceled."); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return slug, nil
}

// restoreCanceledReviewStatus restores the pre-request lifecycle only while the reviewed revision is still current.
func restoreCanceledReviewStatus(
	ctx context.Context,
	tx pgx.Tx,
	pageID int64,
	requestedRevision, currentRevision int,
	previousStatus string,
) error {
	if requestedRevision != currentRevision {
		return nil
	}

	_, err := tx.Exec(ctx, `
UPDATE pages
SET status=$2,updated_at=now()
WHERE id=$1 AND status='draft'`, pageID, previousStatus)

	return err
}

// CanReviewPage reports whether the user is an assigned reviewer for the pending request.
func (s *Store) CanReviewPage(ctx context.Context, slug string, userID int64) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM page_review_requests rr
  JOIN pages p ON p.id=rr.page_id
  WHERE p.slug=$1 AND p.deleted_at IS NULL AND rr.status='pending'
    AND (
      EXISTS(
        SELECT 1
        FROM page_review_request_reviewers rru
        WHERE rru.request_id=rr.id AND rru.user_id=$2
      )
      OR EXISTS(
        SELECT 1
        FROM user_groups ug
        WHERE rr.reviewer_group_id IS NOT NULL
          AND ug.group_id=rr.reviewer_group_id
          AND ug.user_id=$2
      )
    )
)`, slug, userID).Scan(&allowed)

	return allowed, err
}

// DecidePageReview approves a pending request or asks for changes.
func (s *Store) DecidePageReview(
	ctx context.Context,
	id int64,
	expectedSlug string,
	reviewerID int64,
	administrator bool,
	decision string,
	note string,
) (string, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	pageID, currentRevision, err := lockReviewPage(ctx, tx, id, expectedSlug)
	if err != nil {
		return "", err
	}

	var requesterID int64
	var status string
	var requestedRevision int
	var assigned bool
	err = tx.QueryRow(ctx, `
SELECT rr.requested_by,rr.revision_number,rr.status,
       $2 OR EXISTS(
         SELECT 1 FROM page_review_request_reviewers rru
         WHERE rru.request_id=rr.id AND rru.user_id=$3
       ) OR EXISTS(
         SELECT 1 FROM user_groups ug
         WHERE rr.reviewer_group_id IS NOT NULL
           AND ug.group_id=rr.reviewer_group_id AND ug.user_id=$3
       )
FROM page_review_requests rr
WHERE rr.id=$1
FOR UPDATE OF rr`, id, administrator, reviewerID).Scan(
		&requesterID,
		&requestedRevision,
		&status,
		&assigned,
	)
	if err != nil {
		return "", err
	}
	if status == domain.PageReviewStatusSuperseded || requestedRevision != currentRevision {
		return "", domain.ErrStaleReview
	}
	if status != domain.PageReviewStatusPending {
		return "", domain.ErrReviewClosed
	}
	if !assigned {
		return "", domain.ErrForbidden
	}

	if _, err := tx.Exec(ctx, `
UPDATE page_review_requests
SET status=$2,decision_note=$3,reviewed_by=$4,updated_at=now()
WHERE id=$1`, id, decision, note, reviewerID); err != nil {
		return "", mutationError(err)
	}
	if err := markPageReviewApproved(ctx, tx, pageID, decision); err != nil {
		return "", err
	}

	title, err := reviewPageTitle(ctx, tx, pageID)
	if err != nil {
		return "", err
	}
	if err := notifyReviewRequester(ctx, tx, requesterID, reviewerID, title, expectedSlug, decision, note); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}

	return expectedSlug, nil
}

// lockReviewPage locks the page before its review row and returns the current revision.
func lockReviewPage(ctx context.Context, tx pgx.Tx, requestID int64, slug string) (int64, int, error) {
	var pageID int64
	var currentRevision int
	err := tx.QueryRow(ctx, `
SELECT p.id,coalesce((
  SELECT max(revision_number)
  FROM page_revisions
  WHERE page_id=p.id
),0)
FROM pages p
WHERE p.slug=$2 AND p.deleted_at IS NULL
  AND EXISTS(SELECT 1 FROM page_review_requests rr WHERE rr.id=$1 AND rr.page_id=p.id)
FOR UPDATE OF p`, requestID, slug).Scan(&pageID, &currentRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, domain.ErrNotFound
	}

	return pageID, currentRevision, err
}

// reviewPageTitle returns the current title while the caller already holds the page lock.
func reviewPageTitle(ctx context.Context, tx pgx.Tx, pageID int64) (string, error) {
	var title string
	err := tx.QueryRow(ctx, `
SELECT title
FROM pages
WHERE id=$1`, pageID).Scan(&title)

	return title, err
}

// scanPageReviewRequest maps the common review-request query shape into the domain model.
func scanPageReviewRequest(row pgx.Row) (domain.PageReviewRequest, error) {
	var item domain.PageReviewRequest
	err := row.Scan(
		&item.ID,
		&item.PageSlug,
		&item.RevisionNumber,
		&item.RequestedBy,
		&item.RequestedByName,
		&item.ReviewerGroupID,
		&item.ReviewerGroupName,
		&item.ReviewedBy,
		&item.ReviewedByName,
		&item.Status,
		&item.Note,
		&item.DecisionNote,
		&item.PreviousStatus,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	return item, err
}

// pageReviewRequestReviewers returns the explicitly assigned people for one request.
func (s *Store) pageReviewRequestReviewers(ctx context.Context, requestID int64) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role
FROM page_review_request_reviewers rru
JOIN users u ON u.id=rru.user_id
WHERE rru.request_id=$1
ORDER BY lower(u.display_name),lower(u.username),u.id`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := make([]domain.User, 0)
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role); err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, rows.Err()
}

// reviewPageState locks the page and returns the revision and lifecycle used by a new request.
func reviewPageState(ctx context.Context, tx pgx.Tx, slug string) (int64, int, string, error) {
	var pageID int64
	var revisionNumber int
	var status string
	err := tx.QueryRow(ctx, `
SELECT p.id,coalesce((
  SELECT max(r.revision_number)
  FROM page_revisions r
  WHERE r.page_id=p.id
),0),p.status
FROM pages p
WHERE p.slug=$1 AND p.deleted_at IS NULL
FOR UPDATE OF p`, slug).Scan(&pageID, &revisionNumber, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, "", domain.ErrNotFound
	}

	return pageID, revisionNumber, status, err
}

// replacePageReviewers replaces the explicitly assigned users for a pending request.
func replacePageReviewers(ctx context.Context, tx pgx.Tx, requestID int64, reviewerIDs []int64) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM page_review_request_reviewers
WHERE request_id=$1`, requestID); err != nil {
		return err
	}

	for _, userID := range reviewerIDs {
		if _, err := tx.Exec(ctx, `
INSERT INTO page_review_request_reviewers(request_id,user_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, requestID, userID); err != nil {
			return err
		}
	}

	return nil
}

// notifyPageReviewTargets notifies assigned people, a selected group, or administrators as fallback.
func notifyPageReviewTargets(
	ctx context.Context,
	tx pgx.Tx,
	requestID, actorID int64,
	titlePrefix, body string,
) error {
	tag, err := tx.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
SELECT DISTINCT target.user_id,'review',$3 || p.title,$4,'/pages/' || p.slug
FROM page_review_requests rr
JOIN pages p ON p.id=rr.page_id
JOIN LATERAL (
  SELECT rru.user_id
  FROM page_review_request_reviewers rru
  WHERE rru.request_id=rr.id
  UNION
  SELECT ug.user_id
  FROM user_groups ug
  WHERE rr.reviewer_group_id IS NOT NULL AND ug.group_id=rr.reviewer_group_id
) target ON true
JOIN users u ON u.id=target.user_id AND u.enabled AND u.role IN ('admin','editor')
WHERE rr.id=$1 AND target.user_id<>$2`, requestID, actorID, titlePrefix, body)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		return nil
	}

	_, err = tx.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
SELECT u.id,'review',$3 || p.title,$4,'/pages/' || p.slug
FROM page_review_requests rr
JOIN pages p ON p.id=rr.page_id
JOIN users u ON u.role='admin' AND u.enabled
WHERE rr.id=$1 AND u.id<>$2`, requestID, actorID, titlePrefix, body)

	return err
}

// markPageReviewApproved verifies a page only for an approval decision.
func markPageReviewApproved(ctx context.Context, tx pgx.Tx, pageID int64, decision string) error {
	if decision != domain.PageReviewStatusApproved {
		return nil
	}

	_, err := tx.Exec(ctx, `
UPDATE pages
SET status='verified',last_reviewed_at=now(),updated_at=now()
WHERE id=$1`, pageID)

	return err
}

// notifyReviewRequester sends the review result unless the requester reviewed their own request.
func notifyReviewRequester(
	ctx context.Context,
	tx pgx.Tx,
	requesterID, reviewerID int64,
	title, slug, decision, note string,
) error {
	if requesterID == reviewerID {
		return nil
	}

	body := note
	if body == "" {
		body = "The review was " + decision + "."
	}

	_, err := tx.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
VALUES($1,'review',$2,$3,$4)`, requesterID, "Review "+decision+" for "+title, body, "/pages/"+slug)

	return err
}

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

// AddPageReviewComment persists one validated line comment or suggestion on a pending review.
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
	var comment domain.PageReviewComment
	err := s.pool.QueryRow(ctx, `
WITH inserted AS (
  INSERT INTO page_review_comments(
    request_id,user_id,side,start_line,end_line,body,is_suggestion,original_text,replacement_text
  )
  SELECT rr.id,$3,$4,$5,$6,$7,$8,$9,$10
  FROM page_review_requests rr
  JOIN pages p ON p.id=rr.page_id
  WHERE rr.id=$1 AND p.slug=$2 AND p.deleted_at IS NULL AND rr.status='pending'
  RETURNING id,request_id,user_id,side,start_line,end_line,body,is_suggestion,
            original_text,replacement_text,applied_by,applied_at,created_at
)
SELECT i.id,i.request_id,coalesce(i.user_id,0),coalesce(u.display_name,u.username,'Deleted user'),
       i.side,i.start_line,i.end_line,i.body,i.is_suggestion,i.original_text,i.replacement_text,
       coalesce(i.applied_by,0),'',i.applied_at,i.created_at
FROM inserted i
LEFT JOIN users u ON u.id=i.user_id`,
		requestID,
		slug,
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
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageReviewComment{}, domain.ErrReviewClosed
	}
	if err != nil {
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

	resolvedIDs, err := lockOpenReviewSuggestions(ctx, tx, requestID, suggestionIDs)
	if err != nil {
		return domain.Page{}, err
	}
	if !sameReviewSuggestionIDs(suggestionIDs, resolvedIDs) {
		return domain.Page{}, domain.ErrNotFound
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

// lockOpenReviewSuggestions locks selected unapplied suggestions and returns their identifiers.
func lockOpenReviewSuggestions(ctx context.Context, tx pgx.Tx, requestID int64, suggestionIDs []int64) ([]int64, error) {
	rows, err := tx.Query(ctx, `
SELECT id
FROM page_review_comments
WHERE request_id=$1 AND id=ANY($2) AND is_suggestion AND applied_at IS NULL
ORDER BY id
FOR UPDATE`, requestID, suggestionIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	resolved := make([]int64, 0, len(suggestionIDs))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		resolved = append(resolved, id)
	}

	return resolved, rows.Err()
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
