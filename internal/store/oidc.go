package store

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/internal/domain"
)

const (
	pendingOIDCStatusPending  = "pending"
	pendingOIDCStatusRejected = "rejected"
)

// SetExternalAdminStatus records the most recently asserted external administrator state.
func (s *Store) SetExternalAdminStatus(ctx context.Context, userID int64, method string, admin bool) error {
	var query string

	switch method {
	case "oidc":
		query = `UPDATE users SET oidc_admin_observed=true,oidc_external_admin=$2 WHERE id=$1`
	case "trusted-proxy":
		query = `UPDATE users SET trusted_proxy_admin_observed=true,trusted_proxy_external_admin=$2 WHERE id=$1`
	default:
		return domain.NewValidationError("method", "Choose a supported external authentication method.")
	}

	tag, err := s.pool.Exec(ctx, query, userID, admin)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// OIDCUser resolves an existing Kumbuka account by its verified OIDC issuer and subject.
func (s *Store) OIDCUser(ctx context.Context, issuer, subject string) (domain.User, error) {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	if issuer == "" || subject == "" {
		return domain.User{}, domain.ErrNotFound
	}

	var user domain.User
	err := s.pool.QueryRow(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version,u.oidc_external_admin
FROM oidc_identities oi
JOIN users u ON u.id=oi.user_id
WHERE oi.issuer=$1 AND oi.subject=$2 AND u.enabled`, issuer, subject).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion, &user.ExternalAdmin)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}

	return user, err
}

// OIDCIdentities returns all persisted OIDC bindings grouped by their Kumbuka user identifier.
func (s *Store) OIDCIdentities(ctx context.Context) ([]domain.OIDCIdentity, error) {
	rows, err := s.pool.Query(ctx, `
SELECT user_id,issuer,subject,created_at
FROM oidc_identities
ORDER BY user_id,issuer,created_at`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	identities := make([]domain.OIDCIdentity, 0)

	for rows.Next() {
		var identity domain.OIDCIdentity
		if err := rows.Scan(&identity.UserID, &identity.Issuer, &identity.Subject, &identity.CreatedAt); err != nil {
			return nil, err
		}

		identities = append(identities, identity)
	}

	return identities, rows.Err()
}

// OIDCGroupMappings returns configured external-to-Kumbuka group mappings.
func (s *Store) OIDCGroupMappings(ctx context.Context) ([]domain.OIDCGroupMapping, error) {
	rows, err := s.pool.Query(ctx, `
SELECT m.oidc_group,g.id,g.name
FROM oidc_group_mappings m
JOIN wiki_groups g ON g.id=m.group_id
ORDER BY lower(m.oidc_group),m.oidc_group,g.id`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	mappings := make([]domain.OIDCGroupMapping, 0)

	for rows.Next() {
		var mapping domain.OIDCGroupMapping
		if err := rows.Scan(&mapping.OIDCGroup, &mapping.GroupID, &mapping.GroupName); err != nil {
			return nil, err
		}

		mappings = append(mappings, mapping)
	}

	return mappings, rows.Err()
}

// SyncOIDCGroups applies mapped memberships while preserving unrelated manually managed groups.
func (s *Store) SyncOIDCGroups(
	ctx context.Context,
	userID int64,
	claimedGroups []string,
	mappings []domain.OIDCGroupMapping,
	authoritative bool,
) error {
	claimed := make(map[string]bool, len(claimedGroups))

	for _, group := range claimedGroups {
		if group = strings.TrimSpace(group); group != "" {
			claimed[group] = true
		}
	}

	managed := make(map[int64]bool, len(mappings))
	desired := make(map[int64]bool, len(mappings))

	for _, mapping := range mappings {
		if mapping.GroupID <= 0 {
			continue
		}

		managed[mapping.GroupID] = true

		if claimed[strings.TrimSpace(mapping.OIDCGroup)] {
			desired[mapping.GroupID] = true
		}
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if authoritative {
		for groupID := range managed {
			if desired[groupID] {
				continue
			}
			if _, err := tx.Exec(ctx, `
DELETE FROM user_groups
WHERE user_id=$1 AND group_id=$2`, userID, groupID); err != nil {
				return err
			}
		}
	}
	for groupID := range desired {
		if _, err := tx.Exec(ctx, `
INSERT INTO user_groups(user_id,group_id)
VALUES($1,$2)
ON CONFLICT DO NOTHING`, userID, groupID); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// PendingOIDCIdentities returns identities waiting for an administrator decision.
func (s *Store) PendingOIDCIdentities(ctx context.Context) ([]domain.PendingOIDCIdentity, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  p.id,
  p.issuer,
  p.subject,
  p.username,
  p.email,
  p.display_name,
  p.status,
  p.first_seen_at,
  p.last_seen_at,
  coalesce(candidate.id,0),
  coalesce(candidate.username,''),
  coalesce(candidate.display_name,'')
FROM pending_oidc_identities p
LEFT JOIN LATERAL (
  SELECT u.id,u.username,u.display_name
  FROM users u
  WHERE
    (p.email<>'' AND lower(u.email)=lower(p.email))
    OR (p.username<>'' AND lower(u.username)=lower(p.username))
  ORDER BY
    CASE WHEN p.email<>'' AND lower(u.email)=lower(p.email) THEN 0 ELSE 1 END,
    u.id
  LIMIT 1
) candidate ON true
ORDER BY
  CASE p.status WHEN 'pending' THEN 0 ELSE 1 END,
  p.last_seen_at DESC,
  p.id DESC`)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	identities := make([]domain.PendingOIDCIdentity, 0)

	for rows.Next() {
		var identity domain.PendingOIDCIdentity
		if err := rows.Scan(
			&identity.ID,
			&identity.Issuer,
			&identity.Subject,
			&identity.Username,
			&identity.Email,
			&identity.DisplayName,
			&identity.Status,
			&identity.FirstSeenAt,
			&identity.LastSeenAt,
			&identity.SuggestedUserID,
			&identity.SuggestedUsername,
			&identity.SuggestedDisplayName,
		); err != nil {
			return nil, err
		}

		identities = append(identities, identity)
	}

	return identities, rows.Err()
}

// LoginOIDCUser resolves or creates a Kumbuka account for a verified OIDC identity.
func (s *Store) LoginOIDCUser(
	ctx context.Context,
	issuer, subject, username, email, displayName string,
) (domain.User, error) {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if issuer == "" || subject == "" || username == "" {
		return domain.User{}, domain.NewValidationError("identity", "The identity provider must supply issuer, subject, and username.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	user, found, err := oidcUserForUpdate(ctx, tx, issuer, subject)
	if err != nil {
		return domain.User{}, err
	}
	if found {
		user, err = refreshOIDCUser(ctx, tx, user, username, email, displayName)
		if err != nil {
			return domain.User{}, err
		}
		if _, err := tx.Exec(ctx, `
DELETE FROM pending_oidc_identities
WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, err
		}

		return user, nil
	}

	pendingStatus, err := pendingOIDCStatusForUpdate(ctx, tx, issuer, subject)
	if err != nil {
		return domain.User{}, err
	}

	// An administrator may have linked this identity while we waited for the
	// pending-row lock. Recheck the binding before creating another account.
	user, found, err = oidcUserForUpdate(ctx, tx, issuer, subject)
	if err != nil {
		return domain.User{}, err
	}
	if found {
		user, err = refreshOIDCUser(ctx, tx, user, username, email, displayName)
		if err != nil {
			return domain.User{}, err
		}
		if _, err := tx.Exec(ctx, `
DELETE FROM pending_oidc_identities
WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, err
		}

		return user, nil
	}

	if pendingStatus == pendingOIDCStatusRejected {
		if _, err := upsertPendingOIDCIdentity(ctx, tx, issuer, subject, username, email, displayName); err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, err
		}

		return domain.User{}, domain.ErrIdentityRejected
	}

	var registrationEnabled bool
	if err := tx.QueryRow(ctx, `
SELECT allow_user_registration
FROM application_settings
WHERE singleton=true`).Scan(&registrationEnabled); err != nil {
		return domain.User{}, err
	}
	if !registrationEnabled {
		if _, err := upsertPendingOIDCIdentity(ctx, tx, issuer, subject, username, email, displayName); err != nil {
			return domain.User{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return domain.User{}, err
		}

		return domain.User{}, domain.ErrIdentityApprovalRequired
	}

	user, err = createOIDCUser(ctx, tx, issuer, subject, username, email, displayName)
	if err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM pending_oidc_identities
WHERE issuer=$1 AND subject=$2`, issuer, subject); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

// ApprovePendingOIDCIdentity creates a new Kumbuka user for an administrator-approved identity.
func (s *Store) ApprovePendingOIDCIdentity(ctx context.Context, pendingID int64) (domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	pending, err := pendingOIDCIdentityForUpdate(ctx, tx, pendingID)
	if err != nil {
		return domain.User{}, err
	}
	if pending.Status != pendingOIDCStatusPending {
		return domain.User{}, domain.ErrForbidden
	}

	if _, found, err := oidcUserForUpdate(ctx, tx, pending.Issuer, pending.Subject); err != nil {
		return domain.User{}, err
	} else if found {
		return domain.User{}, domain.ErrAlreadyExists
	}

	user, err := createOIDCUser(
		ctx,
		tx,
		pending.Issuer,
		pending.Subject,
		pending.Username,
		pending.Email,
		pending.DisplayName,
	)
	if err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM pending_oidc_identities
WHERE id=$1`, pendingID); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

// LinkPendingOIDCIdentity binds an approved identity to an existing Kumbuka user.
func (s *Store) LinkPendingOIDCIdentity(ctx context.Context, pendingID, userID int64) (domain.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	pending, err := pendingOIDCIdentityForUpdate(ctx, tx, pendingID)
	if err != nil {
		return domain.User{}, err
	}
	if pending.Status != pendingOIDCStatusPending {
		return domain.User{}, domain.ErrForbidden
	}

	if _, found, err := oidcUserForUpdate(ctx, tx, pending.Issuer, pending.Subject); err != nil {
		return domain.User{}, err
	} else if found {
		return domain.User{}, domain.ErrAlreadyExists
	}

	var user domain.User

	if err := tx.QueryRow(ctx, `
SELECT id,username,email,display_name,role
FROM users
WHERE id=$1
FOR UPDATE`, userID).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role); errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	} else if err != nil {
		return domain.User{}, err
	}

	// Replacing the binding invalidates sessions created with the previous
	// subject and blocks that identity from silently registering again.
	var previousSubject string
	err = tx.QueryRow(ctx, `
DELETE FROM oidc_identities
WHERE user_id=$1 AND issuer=$2
RETURNING subject`, userID, pending.Issuer).Scan(&previousSubject)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}

	if previousSubject != "" && previousSubject != pending.Subject {
		if err := rejectOIDCIdentity(
			ctx,
			tx,
			pending.Issuer,
			previousSubject,
			user.Username,
			user.Email,
			user.DisplayName,
		); err != nil {
			return domain.User{}, err
		}
	}

	if _, err := tx.Exec(ctx, `
INSERT INTO oidc_identities(issuer,subject,user_id)
VALUES($1,$2,$3)`, pending.Issuer, pending.Subject, userID); err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `
DELETE FROM pending_oidc_identities
WHERE id=$1`, pendingID); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

// RemoveOIDCIdentity disconnects and blocks one external identity.
func (s *Store) RemoveOIDCIdentity(ctx context.Context, userID int64, issuer, subject string) error {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	var username, email, displayName string

	if err := tx.QueryRow(ctx, `
SELECT username,email,display_name
FROM users
WHERE id=$1
FOR UPDATE`, userID).Scan(&username, &email, &displayName); errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	} else if err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
DELETE FROM oidc_identities
WHERE user_id=$1 AND issuer=$2 AND subject=$3`, userID, issuer, subject)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if err := rejectOIDCIdentity(ctx, tx, issuer, subject, username, email, displayName); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// rejectOIDCIdentity keeps a disconnected subject from automatically registering again.
func rejectOIDCIdentity(
	ctx context.Context,
	tx pgx.Tx,
	issuer, subject, username, email, displayName string,
) error {
	_, err := tx.Exec(ctx, `
INSERT INTO pending_oidc_identities(issuer,subject,username,email,display_name,status)
VALUES($1,$2,$3,$4,$5,'rejected')
ON CONFLICT(issuer,subject) DO UPDATE
SET username=EXCLUDED.username,
    email=EXCLUDED.email,
    display_name=EXCLUDED.display_name,
    status='rejected',
    last_seen_at=now()`, issuer, subject, username, email, displayName)
	return err
}

// SetPendingOIDCIdentityRejected rejects or reopens a pending identity request.
func (s *Store) SetPendingOIDCIdentityRejected(ctx context.Context, pendingID int64, rejected bool) error {
	status := pendingOIDCStatusPending

	if rejected {
		status = pendingOIDCStatusRejected
	}

	tag, err := s.pool.Exec(ctx, `
UPDATE pending_oidc_identities
SET status=$2
WHERE id=$1`, pendingID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return nil
}

// oidcUserForUpdate finds and locks an account bound to one OIDC identity.
func oidcUserForUpdate(ctx context.Context, tx pgx.Tx, issuer, subject string) (user domain.User, found bool, err error) {
	err = tx.QueryRow(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version
FROM oidc_identities oi
JOIN users u ON u.id=oi.user_id
WHERE oi.issuer=$1 AND oi.subject=$2
FOR UPDATE OF u`, issuer, subject).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, false, nil
	}
	if err != nil {
		return domain.User{}, false, err
	}

	return user, true, nil
}

// pendingOIDCIdentityForUpdate returns and locks one administrator-managed identity request.
func pendingOIDCIdentityForUpdate(ctx context.Context, tx pgx.Tx, pendingID int64) (domain.PendingOIDCIdentity, error) {
	var pending domain.PendingOIDCIdentity
	err := tx.QueryRow(ctx, `
SELECT id,issuer,subject,username,email,display_name,status,first_seen_at,last_seen_at
FROM pending_oidc_identities
WHERE id=$1
FOR UPDATE`, pendingID).Scan(
		&pending.ID,
		&pending.Issuer,
		&pending.Subject,
		&pending.Username,
		&pending.Email,
		&pending.DisplayName,
		&pending.Status,
		&pending.FirstSeenAt,
		&pending.LastSeenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PendingOIDCIdentity{}, domain.ErrNotFound
	}

	return pending, err
}

// pendingOIDCStatusForUpdate returns the current request state when one already exists.
func pendingOIDCStatusForUpdate(ctx context.Context, tx pgx.Tx, issuer, subject string) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `
SELECT status
FROM pending_oidc_identities
WHERE issuer=$1 AND subject=$2
FOR UPDATE`, issuer, subject).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}

	return status, err
}

// upsertPendingOIDCIdentity records the latest verified profile without changing an administrator decision.
func upsertPendingOIDCIdentity(
	ctx context.Context,
	tx pgx.Tx,
	issuer, subject, username, email, displayName string,
) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `
INSERT INTO pending_oidc_identities(issuer,subject,username,email,display_name)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(issuer,subject) DO UPDATE
SET username=EXCLUDED.username,
    email=EXCLUDED.email,
    display_name=EXCLUDED.display_name,
    last_seen_at=now()
RETURNING status`, issuer, subject, username, email, displayName).Scan(&status)

	return status, err
}

// refreshOIDCUser updates mutable profile fields for an already-bound OIDC identity.
func refreshOIDCUser(
	ctx context.Context,
	tx pgx.Tx,
	user domain.User,
	username, email, displayName string,
) (domain.User, error) {
	if available, err := usernameAvailable(ctx, tx, username, user.ID); err != nil {
		return domain.User{}, err
	} else if !available {
		return domain.User{}, domain.ErrAlreadyExists
	}

	user.Username = username

	err := tx.QueryRow(ctx, `
UPDATE users
SET username=$2,
    email=$3,
    display_name=$4,
    last_login=CASE WHEN enabled THEN now() ELSE last_login END
WHERE id=$1
RETURNING id,username,email,display_name,role,enabled,session_version`, user.ID, user.Username, email, displayName).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)

	return user, err
}

// createOIDCUser creates a fresh Kumbuka account and binds the verified OIDC identity.
func createOIDCUser(
	ctx context.Context,
	tx pgx.Tx,
	issuer, subject, username, email, displayName string,
) (domain.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return domain.User{}, domain.NewValidationError("username", "The identity provider must supply a username.")
	}

	available, err := usernameAvailable(ctx, tx, username, 0)
	if err != nil {
		return domain.User{}, err
	}
	if !available {
		return domain.User{}, domain.ErrAlreadyExists
	}

	var user domain.User
	if err := tx.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,last_login)
VALUES($1,$2,$3,now())
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	); err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO oidc_identities(issuer,subject,user_id)
VALUES($1,$2,$3)`, issuer, subject, user.ID); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

// usernameAvailable checks username ownership case-insensitively for mention safety.
func usernameAvailable(ctx context.Context, tx pgx.Tx, username string, exceptUserID int64) (bool, error) {
	var available bool
	err := tx.QueryRow(ctx, `
SELECT NOT EXISTS(
  SELECT 1 FROM users
  WHERE lower(username)=lower($1) AND id<>$2
)`, username, exceptUserID).Scan(&available)

	return available, err
}
