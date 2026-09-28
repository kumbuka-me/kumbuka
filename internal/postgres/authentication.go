package postgres

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

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

// RefreshTrustedProxyUser resolves a dedicated trusted-proxy binding and refreshes non-overridden fields.
func (s *Store) RefreshTrustedProxyUser(ctx context.Context, username, email, displayName string) (domain.User, error) {
	displayName = cmp.Or(displayName, username)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var user domain.User
	err = tx.QueryRow(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version
FROM trusted_proxy_identities identity
JOIN users u ON u.id=identity.user_id
WHERE identity.username=$1
FOR UPDATE OF identity,u`, username).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		var retired bool
		if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM retired_trusted_proxy_identities WHERE username=$1)`, username).Scan(&retired); err != nil {
			return domain.User{}, err
		}
		if retired {
			return domain.User{}, domain.ErrRegistrationDisabled
		}
		user, err = adoptLegacyTrustedProxyUser(ctx, tx, username)
	}
	if err != nil {
		return domain.User{}, err
	}

	err = tx.QueryRow(ctx, `
UPDATE trusted_proxy_identities identity
SET email=$2,display_name=$3,last_seen_at=now()
FROM users u
WHERE identity.username=$1 AND u.id=identity.user_id
RETURNING u.id`, username, email, displayName).Scan(&user.ID)
	if err != nil {
		return domain.User{}, err
	}
	err = tx.QueryRow(ctx, `
UPDATE users
SET email=CASE WHEN email_overridden THEN email ELSE $2 END,
    display_name=CASE WHEN display_name_overridden THEN display_name ELSE $3 END,
    profile_source='trusted-proxy',
    last_login=CASE WHEN enabled THEN now() ELSE last_login END
WHERE id=$1
RETURNING id,username,email,display_name,role,enabled,session_version`, user.ID, email, displayName).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
	)
	if err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}

	return user, nil
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
func (s *Store) CreateTrustedProxyUser(ctx context.Context, username, email, displayName string) (domain.User, error) {
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
INSERT INTO users(username,email,display_name,profile_source,last_login)
VALUES($1,$2,$3,'trusted-proxy',now())
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(
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
