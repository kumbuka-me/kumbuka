package store

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageGroups returns the collaboration groups assigned to one page.
func (s *Store) PageGroups(ctx context.Context, pageID int64) ([]domain.Group, error) {
	rows, err := s.pool.Query(ctx, `
SELECT g.id,g.name
FROM wiki_groups g
JOIN page_groups pg ON pg.group_id=g.id
WHERE pg.page_id=$1
ORDER BY lower(g.name),g.id`, pageID)
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

// validateAssignableGroup verifies that a user may select one group for page metadata.
func validateAssignableGroup(ctx context.Context, tx pgx.Tx, groupID int64, user domain.User) error {
	if groupID == 0 {
		return nil
	}
	if groupID < 0 {
		return &domain.GroupAssignmentError{Field: "owner_group_id"}
	}

	var allowed bool

	if user.Role == "admin" {
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM wiki_groups WHERE id=$1)`, groupID).Scan(&allowed); err != nil {
			return err
		}
	} else {
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM user_groups WHERE user_id=$1 AND group_id=$2)`, user.ID, groupID).Scan(&allowed); err != nil {
			return err
		}
	}

	if !allowed {
		return &domain.GroupAssignmentError{Field: "owner_group_id"}
	}

	return nil
}

// replacePageGroups validates and updates page collaboration groups in the active transaction.
func replacePageGroups(ctx context.Context, tx pgx.Tx, pageID int64, groupIDs []int64, user domain.User) error {
	unique := make(map[int64]struct{}, len(groupIDs))

	for _, groupID := range groupIDs {
		if groupID <= 0 {
			return &domain.GroupAssignmentError{Field: "group_ids"}
		}
		if _, exists := unique[groupID]; exists {
			continue
		}

		unique[groupID] = struct{}{}

		var allowed bool

		if user.Role == "admin" {
			if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM wiki_groups WHERE id=$1)`, groupID).Scan(&allowed); err != nil {
				return err
			}
		} else {
			if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM user_groups WHERE user_id=$1 AND group_id=$2)`, user.ID, groupID).Scan(&allowed); err != nil {
				return err
			}
		}

		if !allowed {
			return &domain.GroupAssignmentError{Field: "group_ids"}
		}
	}

	if user.Role == "admin" {
		if _, err := tx.Exec(ctx, `
DELETE FROM page_groups
WHERE page_id=$1`, pageID); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(ctx, `
DELETE FROM page_groups pg
USING user_groups ug
WHERE pg.page_id=$1
  AND pg.group_id=ug.group_id
  AND ug.user_id=$2`, pageID, user.ID); err != nil {
			return err
		}
	}

	for groupID := range unique {
		if _, err := tx.Exec(ctx, `
INSERT INTO page_groups(page_id,group_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, pageID, groupID); err != nil {
			return err
		}
	}

	return nil
}
