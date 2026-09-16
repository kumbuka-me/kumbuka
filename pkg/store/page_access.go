package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageAccess evaluates the nearest path rule set for one user.
func (s *Store) PageAccess(ctx context.Context, path string, userID int64) (domain.PageAccess, error) {
	var access domain.PageAccess
	err := s.pool.QueryRow(ctx, `
WITH nearest AS (
  SELECT path
  FROM page_access_rules
  WHERE path=$1 OR $1 LIKE path || '/%'
  GROUP BY path
  ORDER BY length(path) DESC
  LIMIT 1
)
SELECT
  EXISTS(SELECT 1 FROM nearest),
  EXISTS(
    SELECT 1
    FROM nearest n
    JOIN page_access_rules r ON r.path=n.path
    JOIN user_groups ug ON ug.group_id=r.group_id
    WHERE ug.user_id=$2 AND r.access IN ('view','edit')
  ),
  EXISTS(
    SELECT 1
    FROM nearest n
    JOIN page_access_rules r ON r.path=n.path
    JOIN user_groups ug ON ug.group_id=r.group_id
    WHERE ug.user_id=$2 AND r.access='edit'
  )`, path, userID).Scan(&access.Restricted, &access.CanView, &access.CanEdit)
	return access, err
}

// PageAccessRules returns all configured path rules in deterministic order.
func (s *Store) PageAccessRules(ctx context.Context) ([]domain.PageAccessRule, error) {
	rows, err := s.pool.Query(ctx, `
SELECT r.id,r.path,r.group_id,g.name,r.access,r.created_at,r.updated_at
FROM page_access_rules r
JOIN wiki_groups g ON g.id=r.group_id
ORDER BY r.path,lower(g.name),r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.PageAccessRule
	for rows.Next() {
		var item domain.PageAccessRule
		if err := rows.Scan(&item.ID, &item.Path, &item.GroupID, &item.GroupName, &item.Access, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// SavePageAccessRule creates or replaces one group's access at a path.
func (s *Store) SavePageAccessRule(ctx context.Context, path string, groupID int64, access string) error {
	_, err := s.pool.Exec(ctx, `
INSERT INTO page_access_rules(path,group_id,access)
VALUES($1,$2,$3)
ON CONFLICT(path,group_id) DO UPDATE
SET access=excluded.access,updated_at=now()`, path, groupID, access)
	return mutationError(err)
}

// DeletePageAccessRule removes one path rule.
func (s *Store) DeletePageAccessRule(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM page_access_rules WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}
