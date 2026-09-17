package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Users returns all wiki users with their group memberships.
func (s *Store) Users(ctx context.Context) ([]domain.AdminUser, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  u.id,
  u.username,
  u.email,
  u.display_name,
  u.role,
  u.enabled,
  (u.oidc_admin_observed OR u.trusted_proxy_admin_observed),
  (u.oidc_external_admin OR u.trusted_proxy_external_admin),
  lc.user_id IS NOT NULL,
  coalesce(lc.enabled,false),
  coalesce(u.last_login, u.created_at),
  u.last_login IS NOT NULL,
  coalesce(array_agg(g.name ORDER BY g.name) FILTER (WHERE g.id IS NOT NULL),'{}')
FROM users u
LEFT JOIN local_credentials lc ON lc.user_id=u.id
LEFT JOIN user_groups ug ON ug.user_id=u.id
LEFT JOIN wiki_groups g ON g.id=ug.group_id
GROUP BY u.id,lc.user_id,lc.enabled
ORDER BY lower(u.display_name),lower(u.username),u.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var users []domain.AdminUser

	for rows.Next() {
		var user domain.AdminUser
		if err := rows.Scan(
			&user.User.ID,
			&user.User.Username,
			&user.User.Email,
			&user.User.DisplayName,
			&user.User.Role,
			&user.User.Enabled,
			&user.ExternalAdminObserved,
			&user.ExternalAdmin,
			&user.HasLocalCredential,
			&user.LocalCredentialEnabled,
			&user.LastLogin,
			&user.HasLoggedIn,
			&user.Groups,
		); err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	return users, rows.Err()
}

// UpdateUserAccount commits account, membership, credential, and session changes together.
func (s *Store) UpdateUserAccount(ctx context.Context, input domain.UserAccountUpdate) error {
	userID, role, enabled := input.UserID, input.Role, input.Enabled
	groupIDs, localCredentialEnabled := input.GroupIDs, input.LocalCredentialEnabled

	if !domain.ValidUserRole(role) {
		return domain.NewValidationError("role", "Choose a valid user role.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
UPDATE users
SET role=$2,
    enabled=$3,
    session_version=CASE WHEN enabled AND NOT $3 THEN session_version+1 ELSE session_version END
WHERE id=$1`, userID, role, enabled)
	if err != nil {
		return mutationError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	if !enabled {
		if _, err := tx.Exec(ctx, `
DELETE FROM local_sessions
WHERE user_id=$1`, userID); err != nil {
			return mutationError(err)
		}
	}
	if localCredentialEnabled != nil {
		if _, err := tx.Exec(ctx, `
UPDATE local_credentials
SET enabled=$2,updated_at=now()
WHERE user_id=$1`, userID, *localCredentialEnabled); err != nil {
			return mutationError(err)
		}
		if !*localCredentialEnabled {
			if _, err := tx.Exec(ctx, `
DELETE FROM local_sessions
WHERE user_id=$1`, userID); err != nil {
				return mutationError(err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM user_groups
WHERE user_id=$1`, userID); err != nil {
		return mutationError(err)
	}

	for _, groupID := range groupIDs {
		if _, err := tx.Exec(ctx, `
INSERT INTO user_groups(user_id,group_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, userID, groupID); err != nil {
			return mutationError(err)
		}
	}

	if input.PasswordHash != "" {
		if err := setLocalCredential(ctx, tx, userID, input.PasswordHash); err != nil {
			return mutationError(err)
		}
	}

	return mutationError(tx.Commit(ctx))
}

// RevokeUserSessions invalidates local and OIDC sessions for one account.
func (s *Store) RevokeUserSessions(ctx context.Context, userID int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	tag, err := tx.Exec(ctx, `
UPDATE users
SET session_version=session_version+1
WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM local_sessions
WHERE user_id=$1`, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// User returns a user by database identifier.
func (s *Store) User(ctx context.Context, id int64) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
SELECT id,username,email,display_name,role,enabled,session_version
FROM users
WHERE id=$1`, id).
		Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}

	return user, err
}

// UserGroups returns the collaboration groups assigned to one user.
func (s *Store) UserGroups(ctx context.Context, userID int64) ([]domain.Group, error) {
	rows, err := s.pool.Query(ctx, `
SELECT g.id,g.name
FROM wiki_groups g
JOIN user_groups ug ON ug.group_id=g.id
WHERE ug.user_id=$1
ORDER BY lower(g.name),g.id`, userID)
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

// SearchUsers returns accounts matching a username, display name, or email prefix/substring.
func (s *Store) SearchUsers(ctx context.Context, query string, limit int) ([]domain.User, error) {
	query = strings.TrimSpace(query)

	if limit <= 0 || limit > 50 {
		limit = 20
	}

	rows, err := s.pool.Query(ctx, `
SELECT id,username,email,display_name,role,enabled
FROM users
WHERE username ILIKE $1 OR display_name ILIKE $1 OR email ILIKE $1
ORDER BY
  CASE WHEN username ILIKE $2 OR display_name ILIKE $2 THEN 0 ELSE 1 END,
  lower(display_name),lower(username),id
LIMIT $3`, "%"+query+"%", query+"%", limit)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	users := make([]domain.User, 0)

	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled); err != nil {
			return nil, err
		}

		users = append(users, user)
	}

	return users, rows.Err()
}
