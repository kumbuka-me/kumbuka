package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
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

// ReviewGroups returns only the fields needed by page review target selectors. Unlike Groups, this deliberately avoids membership and page-count aggregates.
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
RETURNING id`, pageID, revisionNumber, actorID, reviewerGroupID, note, string(previousStatus)).Scan(&id)
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

	var status domain.PageReviewStatus
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

	var status domain.PageReviewStatus
	var previousStatus domain.PageStatus
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
	previousStatus domain.PageStatus,
) error {
	if requestedRevision != currentRevision {
		return nil
	}

	_, err := tx.Exec(ctx, `
UPDATE pages
SET status=$2,updated_at=now()
WHERE id=$1 AND status='draft'`, pageID, string(previousStatus))

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

// reviewDecisionState contains the locked review row fields needed to authorize a decision.
type reviewDecisionState struct {
	// requesterID identifies the user who requested the review.
	requesterID int64
	// requestedRevision is the revision captured when the review was opened.
	requestedRevision int
	// status is the current review-request status.
	status domain.PageReviewStatus
	// assigned reports whether the reviewer is authorized for this request.
	assigned bool
}

// DecidePageReview approves a pending request or asks for changes.
func (s *Store) DecidePageReview(
	ctx context.Context,
	id int64,
	expectedSlug string,
	reviewerID int64,
	administrator bool,
	decision domain.PageReviewStatus,
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
	state, err := lockReviewDecision(ctx, tx, id, reviewerID, administrator)
	if err != nil {
		return "", err
	}
	if err := validateReviewDecisionState(state, currentRevision); err != nil {
		return "", err
	}
	if err := persistReviewDecision(ctx, tx, id, pageID, reviewerID, decision, note); err != nil {
		return "", err
	}
	title, err := reviewPageTitle(ctx, tx, pageID)
	if err != nil {
		return "", err
	}
	if err := notifyReviewRequester(ctx, tx, state.requesterID, reviewerID, title, expectedSlug, decision, note); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return expectedSlug, nil
}

// lockReviewDecision locks the review request and resolves reviewer assignment.
func lockReviewDecision(ctx context.Context, tx pgx.Tx, id, reviewerID int64, administrator bool) (reviewDecisionState, error) {
	var state reviewDecisionState
	err := tx.QueryRow(ctx, `
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
		&state.requesterID, &state.requestedRevision, &state.status, &state.assigned,
	)
	return state, err
}

// validateReviewDecisionState verifies freshness, openness, and reviewer assignment.
func validateReviewDecisionState(state reviewDecisionState, currentRevision int) error {
	if state.status == domain.PageReviewStatusSuperseded || state.requestedRevision != currentRevision {
		return domain.ErrStaleReview
	}
	if state.status != domain.PageReviewStatusPending {
		return domain.ErrReviewClosed
	}
	if !state.assigned {
		return domain.ErrForbidden
	}
	return nil
}

// persistReviewDecision updates the review request and page-level approval metadata.
func persistReviewDecision(ctx context.Context, tx pgx.Tx, id, pageID, reviewerID int64, decision domain.PageReviewStatus, note string) error {
	if _, err := tx.Exec(ctx, `
UPDATE page_review_requests
SET status=$2,decision_note=$3,reviewed_by=$4,updated_at=now()
WHERE id=$1`, id, string(decision), note, reviewerID); err != nil {
		return mutationError(err)
	}
	return markPageReviewApproved(ctx, tx, pageID, decision)
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
func reviewPageState(ctx context.Context, tx pgx.Tx, slug string) (int64, int, domain.PageStatus, error) {
	var pageID int64
	var revisionNumber int
	var status domain.PageStatus
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
func markPageReviewApproved(ctx context.Context, tx pgx.Tx, pageID int64, decision domain.PageReviewStatus) error {
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
	title, slug string, decision domain.PageReviewStatus, note string,
) error {
	if requesterID == reviewerID {
		return nil
	}

	body := note
	if body == "" {
		body = "The review was " + string(decision) + "."
	}

	_, err := tx.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
VALUES($1,'review',$2,$3,$4)`, requesterID, "Review "+string(decision)+" for "+title, body, "/pages/"+slug)

	return err
}
