package postgres

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// EnsureAdministrator creates or refreshes the administrator used by no-auth mode.
func (s *Store) EnsureAdministrator(ctx context.Context, username, email, displayName string) (domain.User, error) {
	displayName = cmp.Or(displayName, username)

	var user domain.User
	err := s.pool.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,role,last_login)
VALUES($1,$2,$3,'admin',now())
ON CONFLICT(username) DO UPDATE SET
  email=EXCLUDED.email,
  display_name=EXCLUDED.display_name,
  role='admin',
  enabled=true,
  last_login=now()
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(
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

const trustedProxyActivityRefreshInterval = 5 * time.Minute

// RefreshTrustedProxyUser resolves one trusted-proxy identity. The steady-state path is one statement and performs no writes until provider data, asserted admin state, or the throttled activity timestamp changes.
func (s *Store) RefreshTrustedProxyUser(
	ctx context.Context,
	username, email, displayName string,
	adminObserved, externalAdmin bool,
) (domain.User, error) {
	displayName = cmp.Or(displayName, username)

	user, err := s.refreshTrustedProxyUser(ctx, username, email, displayName, adminObserved, externalAdmin)
	if !errors.Is(err, domain.ErrNotFound) {
		return user, err
	}

	// Older installations may have a trusted-proxy-created local account without
	// the dedicated identity row introduced by migration 014. Adopt it once, then
	// all subsequent requests use the one-statement fast path above.
	if err := s.adoptLegacyTrustedProxyIdentity(ctx, username); err != nil {
		return domain.User{}, err
	}
	return s.refreshTrustedProxyUser(ctx, username, email, displayName, adminObserved, externalAdmin)
}

// refreshTrustedProxyUser returns the current user and conditionally refreshes provider-owned fields in one round trip.
func (s *Store) refreshTrustedProxyUser(
	ctx context.Context,
	username, email, displayName string,
	adminObserved, externalAdmin bool,
) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
WITH current AS MATERIALIZED (
  SELECT
    u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version,
    u.email_overridden,u.display_name_overridden,u.profile_source,
    u.trusted_proxy_admin_observed,u.trusted_proxy_external_admin,
    identity.last_seen_at
  FROM trusted_proxy_identities identity
  JOIN users u ON u.id=identity.user_id
  WHERE identity.username=$1
), identity_update AS (
  UPDATE trusted_proxy_identities identity
  SET email=$2,display_name=$3,last_seen_at=now()
  FROM current
  WHERE identity.username=$1
    AND (
      identity.email IS DISTINCT FROM $2 OR
      identity.display_name IS DISTINCT FROM $3 OR
      current.last_seen_at < now() - ($6 * interval '1 second')
    )
  RETURNING identity.user_id
), user_update AS (
  UPDATE users u
  SET email=CASE WHEN u.email_overridden THEN u.email ELSE $2 END,
      display_name=CASE WHEN u.display_name_overridden THEN u.display_name ELSE $3 END,
      profile_source='trusted-proxy',
      last_login=CASE
        WHEN u.enabled AND current.last_seen_at < now() - ($6 * interval '1 second') THEN now()
        ELSE u.last_login
      END,
      trusted_proxy_admin_observed=CASE WHEN $4 THEN true ELSE u.trusted_proxy_admin_observed END,
      trusted_proxy_external_admin=CASE WHEN $4 THEN $5 ELSE u.trusted_proxy_external_admin END
  FROM current
  WHERE u.id=current.id
    AND (
      (NOT u.email_overridden AND u.email IS DISTINCT FROM $2) OR
      (NOT u.display_name_overridden AND u.display_name IS DISTINCT FROM $3) OR
      u.profile_source IS DISTINCT FROM 'trusted-proxy' OR
      ($4 AND (NOT u.trusted_proxy_admin_observed OR u.trusted_proxy_external_admin IS DISTINCT FROM $5)) OR
      current.last_seen_at < now() - ($6 * interval '1 second')
    )
  RETURNING u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version
)
SELECT id,username,email,display_name,role,enabled,session_version
FROM user_update
UNION ALL
SELECT id,username,email,display_name,role,enabled,session_version
FROM current
WHERE NOT EXISTS (SELECT 1 FROM user_update)
LIMIT 1`,
		username,
		email,
		displayName,
		adminObserved,
		externalAdmin,
		int64(trustedProxyActivityRefreshInterval/time.Second),
	).Scan(
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

// adoptLegacyTrustedProxyIdentity creates the dedicated identity row for one pre-migration account when present.
func (s *Store) adoptLegacyTrustedProxyIdentity(ctx context.Context, username string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var retired bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM retired_trusted_proxy_identities WHERE username=$1)`, username).Scan(&retired); err != nil {
		return err
	}
	if retired {
		return domain.ErrRegistrationDisabled
	}

	if _, err := adoptLegacyTrustedProxyUser(ctx, tx, username); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// adoptLegacyTrustedProxyUser creates a binding for an existing pre-migration proxy account by username only.
func adoptLegacyTrustedProxyUser(ctx context.Context, tx pgx.Tx, username string) (domain.User, error) {
	var user domain.User
	err := tx.QueryRow(ctx, `
SELECT id,username,email,display_name,role,enabled,session_version
FROM users
WHERE username=$1 AND profile_source='local'
  AND NOT EXISTS (SELECT 1 FROM trusted_proxy_identities identity WHERE identity.user_id=users.id)
FOR UPDATE`, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO trusted_proxy_identities(username,user_id,email,display_name)
VALUES($1,$2,$3,$4)`, username, user.ID, user.Email, user.DisplayName)
	return user, err
}

// CreateTrustedProxyUser creates one account and a separate trusted-proxy identity binding.
func (s *Store) CreateTrustedProxyUser(
	ctx context.Context,
	username, email, displayName string,
	adminObserved, externalAdmin bool,
) (domain.User, error) {
	displayName = cmp.Or(displayName, username)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var retired bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM retired_trusted_proxy_identities WHERE username=$1)`, username).Scan(&retired); err != nil {
		return domain.User{}, err
	}
	if retired {
		return domain.User{}, domain.ErrRegistrationDisabled
	}
	var user domain.User
	err = tx.QueryRow(ctx, `
INSERT INTO users(
  username,email,display_name,profile_source,last_login,
  trusted_proxy_admin_observed,trusted_proxy_external_admin
)
VALUES($1,$2,$3,'trusted-proxy',now(),$4,$5)
RETURNING id,username,email,display_name,role,enabled,session_version`,
		username, email, displayName, adminObserved, externalAdmin,
	).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)
	if err != nil {
		return domain.User{}, mutationError(err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO trusted_proxy_identities(username,user_id,email,display_name)
VALUES($1,$2,$3,$4)`, username, user.ID, email, displayName); err != nil {
		return domain.User{}, mutationError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, mutationError(err)
	}
	return user, nil
}

// UserByToken authenticates an API bearer token and updates its last-used timestamp.
func (s *Store) UserByToken(ctx context.Context, token string) (domain.User, error) {
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])
	var u domain.User
	err := s.pool.QueryRow(ctx, `
UPDATE api_tokens t
SET last_used=now() FROM users u
WHERE t.token_hash=$1 AND coalesce(t.user_id,t.created_by)=u.id AND u.enabled AND (t.expires_at IS NULL OR t.expires_at>now())
RETURNING u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version`, hash).
		Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.Role, &u.Enabled, &u.SessionVersion)

	if errors.Is(err, pgx.ErrNoRows) {
		err = domain.ErrNotFound
	}

	return u, err
}
