package postgres

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageWatch returns the current user's watch scope for an exact page path.
func (s *Store) PageWatch(ctx context.Context, slug string, userID int64) (domain.PageWatch, error) {
	var watch domain.PageWatch
	err := s.pool.QueryRow(ctx, `
SELECT path,scope,created_at
FROM page_watches
WHERE user_id=$1 AND path=$2`, userID, strings.Trim(strings.TrimSpace(slug), "/")).Scan(
		&watch.Path,
		&watch.Scope,
		&watch.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PageWatch{}, nil
	}
	return watch, err
}

// SetPageWatch creates, changes, or removes one user's exact-path watch.
func (s *Store) SetPageWatch(ctx context.Context, slug string, userID int64, scope string) error {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	scope = strings.TrimSpace(scope)
	if slug == "" || userID <= 0 {
		return domain.NewValidationError("watch", "Choose a valid page to watch.")
	}
	if scope == "" {
		_, err := s.pool.Exec(ctx, `
DELETE FROM page_watches
WHERE user_id=$1 AND path=$2`, userID, slug)
		return err
	}
	if scope != domain.PageWatchScopePage && scope != domain.PageWatchScopeSubtree {
		return domain.NewValidationError("watch", "Choose page or subtree notifications.")
	}
	_, err := s.pool.Exec(ctx, `
INSERT INTO page_watches(user_id,path,scope)
VALUES($1,$2,$3)
ON CONFLICT(user_id,path) DO UPDATE
SET scope=EXCLUDED.scope,created_at=now()`, userID, slug, scope)
	return err
}

// NotifyPageWatchers creates one notification per matching watcher, excluding the actor.
func (s *Store) NotifyPageWatchers(ctx context.Context, actorID int64, slug, title, body, url string) error {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	_, err := s.pool.Exec(ctx, `
INSERT INTO notifications(user_id,kind,title,body,url)
SELECT DISTINCT w.user_id,'watch',$3,$4,$5
FROM page_watches w
JOIN users u ON u.id=w.user_id AND u.enabled
WHERE w.user_id<>$1
  AND (
    (w.scope='page' AND w.path=$2)
    OR
    (w.scope='subtree' AND ($2=w.path OR $2 LIKE w.path || '/%'))
  )`, actorID, slug, title, body, url)
	return err
}
