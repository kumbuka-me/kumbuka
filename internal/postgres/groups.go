package postgres

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Groups returns all groups and their current user and page counts. Aggregate each relation independently so memberships and page assignments do not form a multiplicative join before counting.
func (s *Store) Groups(ctx context.Context) ([]domain.Group, error) {
	rows, err := s.importQuery(ctx).Query(ctx, `
SELECT
  g.id,
  g.name,
  coalesce(ug.user_count,0),
  coalesce(pg.page_count,0)
FROM wiki_groups g
LEFT JOIN (
  SELECT group_id,count(*) AS user_count
  FROM user_groups
  GROUP BY group_id
) ug ON ug.group_id=g.id
LEFT JOIN (
  SELECT pg.group_id,count(*) AS page_count
  FROM page_groups pg
  JOIN pages p ON p.id=pg.page_id AND p.deleted_at IS NULL
  GROUP BY pg.group_id
) pg ON pg.group_id=g.id
ORDER BY lower(g.name),g.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var groups []domain.Group

	for rows.Next() {
		var group domain.Group
		if err := rows.Scan(&group.ID, &group.Name, &group.UserCount, &group.PageCount); err != nil {
			return nil, err
		}

		groups = append(groups, group)
	}

	return groups, rows.Err()
}

// CreateGroup creates a normalized group and returns its persisted record.
func (s *Store) CreateGroup(ctx context.Context, name string) (domain.Group, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Group{}, domain.NewValidationError("name", "A group name is required.")
	}

	var group domain.Group
	err := s.importQuery(ctx).QueryRow(ctx, `
INSERT INTO wiki_groups(name)
VALUES($1)
RETURNING id,name`, name).
		Scan(&group.ID, &group.Name)

	return group, mutationError(err)
}

// DeleteGroup removes a group and all of its user memberships.
func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM wiki_groups
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// AssignableGroups returns all groups for admins and memberships for other users.
func (s *Store) AssignableGroups(ctx context.Context, user domain.User) ([]domain.Group, error) {
	if user.IsAdministrator() {
		return s.Groups(ctx)
	}
	return s.UserGroups(ctx, user.ID)
}

// GroupMembers returns accounts assigned to a group.
func (s *Store) GroupMembers(ctx context.Context, groupID int64) ([]domain.User, error) {
	rows, err := s.pool.Query(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role
FROM users u
JOIN user_groups ug ON ug.user_id=u.id
WHERE ug.group_id=$1
ORDER BY lower(u.display_name),lower(u.username),u.id`, groupID)
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

// AddGroupMember assigns a user to a group.
func (s *Store) AddGroupMember(ctx context.Context, groupID, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
INSERT INTO user_groups(user_id,group_id)
SELECT u.id,g.id FROM users u CROSS JOIN wiki_groups g
WHERE u.id=$2 AND g.id=$1
ON CONFLICT DO NOTHING`, groupID, userID)

	if err == nil && tag.RowsAffected() == 0 {
		var exists bool
		err = s.pool.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM user_groups WHERE user_id=$2 AND group_id=$1)`, groupID, userID).
			Scan(&exists)
		if err == nil && !exists {
			return domain.ErrNotFound
		}
	}

	return err
}

// RemoveGroupMember removes a user from a group.
func (s *Store) RemoveGroupMember(ctx context.Context, groupID, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM user_groups
WHERE group_id=$1 AND user_id=$2`, groupID, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
