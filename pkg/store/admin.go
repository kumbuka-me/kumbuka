package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Stats returns high-level database counts for administrators.
func (s *Store) Stats(ctx context.Context) (domain.AdminStats, error) {
	var stats domain.AdminStats
	err := s.pool.QueryRow(ctx, `
SELECT
  (SELECT count(*) FROM users),
  (SELECT count(*) FROM wiki_groups),
  (SELECT count(*) FROM pages WHERE deleted_at IS NULL),
  (SELECT count(*) FROM pages WHERE deleted_at IS NOT NULL),
  (SELECT count(*) FROM tags),
  (SELECT count(*) FROM images),
  (SELECT count(*) FROM api_tokens)`).Scan(
		&stats.Users,
		&stats.Groups,
		&stats.Pages,
		&stats.DeletedPages,
		&stats.Tags,
		&stats.Images,
		&stats.Tokens,
	)

	return stats, err
}

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

// UpdateUser updates an account role, enabled state, local-login state, and group memberships transactionally.
func (s *Store) UpdateUser(
	ctx context.Context,
	userID int64,
	role string,
	enabled bool,
	groupIDs []int64,
	localCredentialEnabled *bool,
) error {
	return s.UpdateUserAccount(ctx, domain.UserAccountUpdate{UserID: userID, Role: role, Enabled: enabled, GroupIDs: groupIDs, LocalCredentialEnabled: localCredentialEnabled})
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

// Groups returns all groups and their current user and page counts. Aggregate
// each relation independently so memberships and page assignments do not form
// a multiplicative join before counting.
func (s *Store) Groups(ctx context.Context) ([]domain.Group, error) {
	rows, err := s.pool.Query(ctx, `
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
	err := s.pool.QueryRow(ctx, `
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

// TagInfos returns all stored tags with page usage counts.
func (s *Store) TagInfos(ctx context.Context) ([]domain.TagInfo, error) {
	rows, err := s.pool.Query(ctx, `
SELECT t.id,t.name,count(tp.id)
FROM tags t
LEFT JOIN page_tags pt ON pt.tag_id=t.id
LEFT JOIN pages tp ON tp.id=pt.page_id AND tp.deleted_at IS NULL
GROUP BY t.id
ORDER BY lower(t.name),t.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var tags []domain.TagInfo

	for rows.Next() {
		var tag domain.TagInfo
		if err := rows.Scan(&tag.ID, &tag.Name, &tag.PageCount); err != nil {
			return nil, err
		}

		tags = append(tags, tag)
	}

	return tags, rows.Err()
}

// DeleteTag removes a tag and its page associations.
func (s *Store) DeleteTag(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM tags
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
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

// AssignableGroups returns all groups for admins and memberships for other users.
func (s *Store) AssignableGroups(ctx context.Context, user domain.User) ([]domain.Group, error) {
	if user.Role == "admin" {
		return s.Groups(ctx)
	}
	return s.UserGroups(ctx, user.ID)
}

// ApplicationSettings returns the persisted application-wide settings.
func (s *Store) ApplicationSettings(ctx context.Context) (domain.ApplicationSettings, error) {
	var settings domain.ApplicationSettings
	var externalLinks json.RawMessage
	err := s.pool.QueryRow(ctx, `
SELECT
  allow_user_registration,
  discussions_enabled,
  pdf_url,
  external_links,
  default_typography_size,
	  content_language,
  robots_policy,
  auth_mode,
  oidc_issuer,
  oidc_client_id,
  oidc_group_claim,
  oidc_group_sync,
  oidc_groups_authoritative,
	oidc_admin_group,
  trusted_username_headers,
  trusted_email_headers,
  trusted_display_name_headers,
	trusted_group_headers,
	trusted_admin_group
FROM application_settings
WHERE singleton=true`).Scan(
		&settings.AllowUserRegistration,
		&settings.DiscussionsEnabled,
		&settings.PDFURL,
		&externalLinks,
		&settings.Rendering.DefaultTypographySize,
		&settings.ContentLanguage,
		&settings.RobotsPolicy,
		&settings.Authentication.Mode,
		&settings.Authentication.OIDCIssuer,
		&settings.Authentication.OIDCClientID,
		&settings.Authentication.OIDCGroupClaim,
		&settings.Authentication.OIDCGroupSync,
		&settings.Authentication.OIDCGroupsAuthoritative,
		&settings.Authentication.OIDCAdminGroup,
		&settings.Authentication.TrustedUsernameHeaders,
		&settings.Authentication.TrustedEmailHeaders,
		&settings.Authentication.TrustedDisplayNameHeaders,
		&settings.Authentication.TrustedGroupHeaders,
		&settings.Authentication.TrustedAdminGroup,
	)
	if err != nil {
		return domain.ApplicationSettings{}, err
	}
	if err := json.Unmarshal(externalLinks, &settings.ExternalLinks); err != nil {
		return domain.ApplicationSettings{}, err
	}

	return settings, nil
}

// SaveApplicationSettings updates mutable application-wide settings.
func (s *Store) SaveApplicationSettings(ctx context.Context, settings domain.ApplicationSettings) error {
	externalLinks, err := json.Marshal(settings.ExternalLinks)
	if err != nil {
		return err
	}

	_, err = s.pool.Exec(ctx, `
INSERT INTO application_settings(singleton,allow_user_registration,discussions_enabled,external_links,default_typography_size,content_language,robots_policy,updated_at)
VALUES(true,$1,$2,$3::jsonb,$4,$5,$6,now())
ON CONFLICT(singleton) DO UPDATE
SET allow_user_registration=EXCLUDED.allow_user_registration,
    discussions_enabled=EXCLUDED.discussions_enabled,
    external_links=EXCLUDED.external_links,
    default_typography_size=EXCLUDED.default_typography_size,
    content_language=EXCLUDED.content_language,
    robots_policy=EXCLUDED.robots_policy,
    updated_at=now()`, settings.AllowUserRegistration, settings.DiscussionsEnabled, string(externalLinks), settings.Rendering.DefaultTypographySize, settings.ContentLanguage, settings.RobotsPolicy)
	return err
}

// PDFHeaders returns the configured request headers for the external PDF service.
func (s *Store) PDFHeaders(ctx context.Context) ([]domain.PDFHeader, error) {
	rows, err := s.pool.Query(ctx, `
SELECT id,name,value,sensitive
FROM pdf_headers
ORDER BY lower(name),id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	headers := make([]domain.PDFHeader, 0)
	for rows.Next() {
		var header domain.PDFHeader
		if err := rows.Scan(&header.ID, &header.Name, &header.Value, &header.Sensitive); err != nil {
			return nil, err
		}

		headers = append(headers, header)
	}

	return headers, rows.Err()
}

// SavePDFSettings updates the persisted HTML-to-PDF endpoint and request headers atomically.
func (s *Store) SavePDFSettings(ctx context.Context, pdfURL string, headers []domain.PDFHeader) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
UPDATE application_settings
SET pdf_url=$1,
    updated_at=now()
WHERE singleton=true`, pdfURL); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM pdf_headers`); err != nil {
		return err
	}

	for _, header := range headers {
		if _, err := tx.Exec(ctx, `
INSERT INTO pdf_headers(name,value,sensitive)
VALUES($1,$2,$3)`, header.Name, header.Value, header.Sensitive); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// SaveAuthenticationSettings updates non-secret browser authentication settings.
func (s *Store) SaveAuthenticationSettings(ctx context.Context, settings domain.AuthenticationSettings) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return mutationError(err)
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// A changed assertion source invalidates previously observed external role state.
	if _, err := tx.Exec(ctx, `
UPDATE users
SET oidc_admin_observed=false,oidc_external_admin=false
WHERE EXISTS (
  SELECT 1
  FROM application_settings
  WHERE singleton=true
    AND (oidc_admin_group IS DISTINCT FROM $1 OR oidc_group_claim IS DISTINCT FROM $2)
)`, settings.OIDCAdminGroup, settings.OIDCGroupClaim); err != nil {
		return mutationError(err)
	}
	if _, err := tx.Exec(ctx, `
UPDATE users
SET trusted_proxy_admin_observed=false,trusted_proxy_external_admin=false
WHERE EXISTS (
  SELECT 1
  FROM application_settings
  WHERE singleton=true
    AND (trusted_admin_group IS DISTINCT FROM $1 OR trusted_group_headers IS DISTINCT FROM $2)
)`, settings.TrustedAdminGroup, settings.TrustedGroupHeaders); err != nil {
		return mutationError(err)
	}

	if _, err := tx.Exec(ctx, `
UPDATE application_settings
SET auth_mode=$1,
    oidc_issuer=$2,
    oidc_client_id=$3,
    oidc_group_claim=$4,
    oidc_group_sync=$5,
    oidc_groups_authoritative=$6,
	oidc_admin_group=$7,
    trusted_username_headers=$8,
    trusted_email_headers=$9,
    trusted_display_name_headers=$10,
	trusted_group_headers=$11,
	trusted_admin_group=$12,
    updated_at=now()
WHERE singleton=true`,
		settings.Mode,
		settings.OIDCIssuer,
		settings.OIDCClientID,
		settings.OIDCGroupClaim,
		settings.OIDCGroupSync,
		settings.OIDCGroupsAuthoritative,
		settings.OIDCAdminGroup,
		settings.TrustedUsernameHeaders,
		settings.TrustedEmailHeaders,
		settings.TrustedDisplayNameHeaders,
		settings.TrustedGroupHeaders,
		settings.TrustedAdminGroup,
	); err != nil {
		return mutationError(err)
	}

	if _, err := tx.Exec(ctx, `
DELETE FROM oidc_group_mappings`); err != nil {
		return mutationError(err)
	}

	for _, mapping := range settings.OIDCGroupMappings {
		if _, err := tx.Exec(ctx, `
INSERT INTO oidc_group_mappings(oidc_group,group_id)
VALUES($1,$2)`, strings.TrimSpace(mapping.OIDCGroup), mapping.GroupID); err != nil {
			return mutationError(err)
		}
	}

	return mutationError(tx.Commit(ctx))
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
