package postgres

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
  coalesce(memberships.names,'{}'),
  u.profile_source,
  u.username_overridden,
  u.email_overridden,
  u.display_name_overridden,
  CASE WHEN u.profile_source='oidc' THEN coalesce(oi.username,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.username,'') ELSE '' END,
  CASE WHEN u.profile_source='oidc' THEN coalesce(oi.email,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.email,'') ELSE '' END,
  CASE WHEN u.profile_source='oidc' THEN coalesce(oi.display_name,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.display_name,'') ELSE '' END,
  coalesce(tp.username,'')
FROM users u
LEFT JOIN local_credentials lc ON lc.user_id=u.id
LEFT JOIN LATERAL (
  SELECT array_agg(g.name ORDER BY g.name) AS names
  FROM user_groups ug JOIN wiki_groups g ON g.id=ug.group_id
  WHERE ug.user_id=u.id
) memberships ON true
LEFT JOIN LATERAL (
  SELECT username,email,display_name
  FROM oidc_identities
  WHERE user_id=u.id
  ORDER BY last_seen_at DESC,created_at DESC
  LIMIT 1
) oi ON true
LEFT JOIN trusted_proxy_identities tp ON tp.user_id=u.id
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
			&user.Profile.Source,
			&user.Profile.UsernameOverridden,
			&user.Profile.EmailOverridden,
			&user.Profile.DisplayNameOverridden,
			&user.Profile.ProviderUsername,
			&user.Profile.ProviderEmail,
			&user.Profile.ProviderDisplayName,
			&user.Profile.TrustedProxyUsername,
		); err != nil {
			return nil, err
		}

		user.Profile.UserID = user.User.ID
		user.Profile.Username = user.User.Username
		user.Profile.Email = user.User.Email
		user.Profile.DisplayName = user.User.DisplayName
		users = append(users, user)
	}

	return users, rows.Err()
}

// UserProfile returns the profile source, current values, override state, and latest provider values.
func (s *Store) UserProfile(ctx context.Context, userID int64) (domain.UserProfile, error) {
	return userProfile(ctx, s.pool, userID, false)
}

// RelinkTrustedProxyIdentity atomically replaces one proxy key and the account username.
func (s *Store) RelinkTrustedProxyIdentity(ctx context.Context, userID int64, username string) error {
	username = strings.TrimSpace(username)
	if username == "" {
		return domain.NewValidationError("trusted_proxy_username", "Enter a trusted-proxy username.")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	profile, err := userProfile(ctx, tx, userID, true)
	if err != nil {
		return err
	}
	if profile.Source != domain.ProfileSourceTrustedProxy || profile.TrustedProxyUsername == "" {
		return domain.NewValidationError("trusted_proxy_username", "This account has no trusted-proxy identity to relink.")
	}
	available, err := usernameAvailable(ctx, tx, username, userID)
	if err != nil {
		return err
	}
	if !available {
		return domain.ErrAlreadyExists
	}
	var bound bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM trusted_proxy_identities WHERE username=$1 AND user_id<>$2)`, username, userID).Scan(&bound); err != nil {
		return err
	}
	if bound {
		return domain.ErrAlreadyExists
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM retired_trusted_proxy_identities WHERE username=$1`, username); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO retired_trusted_proxy_identities(username,user_id)
VALUES($1,$2)
ON CONFLICT(username) DO UPDATE SET user_id=EXCLUDED.user_id,retired_at=now()`, profile.TrustedProxyUsername, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
UPDATE trusted_proxy_identities
SET username=$2
WHERE user_id=$1`, userID, username); err != nil {
		return mutationError(err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE users
SET username=$2,username_overridden=false
WHERE id=$1`, userID, username); err != nil {
		return mutationError(err)
	}
	return mutationError(tx.Commit(ctx))
}

// UpdateUserAccount commits profile, account, membership, credential, and session changes together.
func (s *Store) UpdateUserAccount(ctx context.Context, input domain.UserAccountUpdate) error {
	if !domain.ValidUserRole(input.Role) {
		return domain.NewValidationError("role", "Choose a valid user role.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := updateUserAccountRecord(ctx, tx, input); err != nil {
		return mutationError(err)
	}
	if err := updateUserCredentialState(ctx, tx, input); err != nil {
		return mutationError(err)
	}
	if err := replaceUserGroups(ctx, tx, input.UserID, input.GroupIDs); err != nil {
		return mutationError(err)
	}
	if input.PasswordHash != "" {
		if err := setLocalCredential(ctx, tx, input.UserID, input.PasswordHash); err != nil {
			return mutationError(err)
		}
	}
	return mutationError(tx.Commit(ctx))
}

// updateUserAccountRecord updates optional profile fields, role/enabled state, and invalidates sessions on disable.
func updateUserAccountRecord(ctx context.Context, tx pgx.Tx, input domain.UserAccountUpdate) error {
	profile, err := userProfile(ctx, tx, input.UserID, true)
	if err != nil {
		return err
	}
	if profile.Source == domain.ProfileSourceTrustedProxy && (input.Username != nil || input.RevertUsername) {
		return domain.NewValidationError("username", "Relink the trusted-proxy identity to change its username.")
	}
	if profile.Source == domain.ProfileSourceLocal && (input.RevertUsername || input.RevertEmail || input.RevertDisplayName) {
		return domain.NewValidationError("profile", "Local profile fields are not provider-managed.")
	}

	username, email, displayName := profile.Username, profile.Email, profile.DisplayName
	usernameOverridden := profile.UsernameOverridden
	emailOverridden := profile.EmailOverridden
	displayNameOverridden := profile.DisplayNameOverridden
	if input.RevertUsername {
		username, usernameOverridden = profile.ProviderUsername, false
	} else if input.Username != nil {
		username = strings.TrimSpace(*input.Username)
		usernameOverridden = profile.Source != domain.ProfileSourceLocal
	}
	if input.RevertEmail {
		email, emailOverridden = profile.ProviderEmail, false
	} else if input.Email != nil {
		email = strings.TrimSpace(*input.Email)
		emailOverridden = profile.Source != domain.ProfileSourceLocal
	}
	if input.RevertDisplayName {
		displayName, displayNameOverridden = profile.ProviderDisplayName, false
	} else if input.DisplayName != nil {
		displayName = strings.TrimSpace(*input.DisplayName)
		displayNameOverridden = profile.Source != domain.ProfileSourceLocal
	}
	if username == "" {
		return domain.NewValidationError("username", "Username is required.")
	}
	available, err := usernameAvailable(ctx, tx, username, input.UserID)
	if err != nil {
		return err
	}
	if !available {
		return domain.ErrAlreadyExists
	}

	tag, err := tx.Exec(ctx, `
UPDATE users
SET username=$2,
    email=$3,
    display_name=$4,
    username_overridden=$5,
    email_overridden=$6,
    display_name_overridden=$7,
    role=$8,
    enabled=$9,
    session_version=CASE WHEN enabled AND NOT $9 THEN session_version+1 ELSE session_version END
WHERE id=$1`,
		input.UserID,
		username,
		email,
		displayName,
		usernameOverridden,
		emailOverridden,
		displayNameOverridden,
		string(input.Role),
		input.Enabled,
	)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if input.Enabled {
		return nil
	}
	return deleteLocalSessions(ctx, tx, input.UserID)
}

type userProfileQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// userProfile reads one account and the latest metadata from its active external profile source.
func userProfile(ctx context.Context, queryer userProfileQuerier, userID int64, lock bool) (domain.UserProfile, error) {
	query := `
SELECT u.id,u.profile_source,u.username,u.email,u.display_name,
       u.username_overridden,u.email_overridden,u.display_name_overridden,
       CASE WHEN u.profile_source='oidc' THEN coalesce(oi.username,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.username,'') ELSE '' END,
       CASE WHEN u.profile_source='oidc' THEN coalesce(oi.email,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.email,'') ELSE '' END,
       CASE WHEN u.profile_source='oidc' THEN coalesce(oi.display_name,'') WHEN u.profile_source='trusted-proxy' THEN coalesce(tp.display_name,'') ELSE '' END,
       coalesce(tp.username,'')
FROM users u
LEFT JOIN LATERAL (
  SELECT username,email,display_name FROM oidc_identities
  WHERE user_id=u.id ORDER BY last_seen_at DESC,created_at DESC LIMIT 1
) oi ON true
LEFT JOIN trusted_proxy_identities tp ON tp.user_id=u.id
WHERE u.id=$1`
	if lock {
		query += " FOR UPDATE OF u"
	}
	var profile domain.UserProfile
	err := queryer.QueryRow(ctx, query, userID).Scan(
		&profile.UserID, &profile.Source, &profile.Username, &profile.Email, &profile.DisplayName,
		&profile.UsernameOverridden, &profile.EmailOverridden, &profile.DisplayNameOverridden,
		&profile.ProviderUsername, &profile.ProviderEmail, &profile.ProviderDisplayName,
		&profile.TrustedProxyUsername,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserProfile{}, domain.ErrNotFound
	}
	return profile, err
}

// updateUserCredentialState updates the optional local-credential enablement and sessions.
func updateUserCredentialState(ctx context.Context, tx pgx.Tx, input domain.UserAccountUpdate) error {
	if input.LocalCredentialEnabled == nil {
		return nil
	}
	if _, err := tx.Exec(ctx, `
UPDATE local_credentials
SET enabled=$2,updated_at=now()
WHERE user_id=$1`, input.UserID, *input.LocalCredentialEnabled); err != nil {
		return err
	}
	if *input.LocalCredentialEnabled {
		return nil
	}
	return deleteLocalSessions(ctx, tx, input.UserID)
}

// deleteLocalSessions removes all local sessions for one user inside a transaction.
func deleteLocalSessions(ctx context.Context, tx pgx.Tx, userID int64) error {
	_, err := tx.Exec(ctx, `
DELETE FROM local_sessions
WHERE user_id=$1`, userID)
	return err
}

// replaceUserGroups replaces all explicit group memberships for one user.
func replaceUserGroups(ctx context.Context, tx pgx.Tx, userID int64, groupIDs []int64) error {
	if _, err := tx.Exec(ctx, `
DELETE FROM user_groups
WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, groupID := range groupIDs {
		if _, err := tx.Exec(ctx, `
INSERT INTO user_groups(user_id,group_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, userID, groupID); err != nil {
			return err
		}
	}
	return nil
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

// SearchPublicUsers returns enabled accounts matched only by username or display name.
func (s *Store) SearchPublicUsers(ctx context.Context, query string, limit int) ([]domain.User, error) {
	query = strings.TrimSpace(strings.TrimPrefix(query, "@"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
SELECT id,username,email,display_name,role,enabled
FROM users
WHERE enabled AND (username ILIKE $1 OR display_name ILIKE $1)
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

// UserByUsername returns one account by case-insensitive username.
func (s *Store) UserByUsername(ctx context.Context, username string) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
SELECT id,username,email,display_name,role,enabled,session_version
FROM users
WHERE lower(username)=lower($1)`, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	return user, err
}
