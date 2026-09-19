package store

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

// TrustedProxyUser refreshes a trusted-proxy user by username or creates one when registration is enabled.
func (s *Store) TrustedProxyUser(ctx context.Context, username, email, displayName string) (domain.User, error) {
	displayName = cmp.Or(displayName, username)

	var user domain.User
	err := s.pool.QueryRow(ctx, `
UPDATE users
SET email=$2,
    display_name=$3,
    last_login=CASE WHEN enabled THEN now() ELSE last_login END
WHERE username=$1
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, err
	}

	registrationEnabled, overridden := s.userRegistrationOverride()
	if !overridden {
		settings, err := s.ApplicationSettings(ctx)
		if err != nil {
			return domain.User{}, err
		}

		registrationEnabled = settings.AllowUserRegistration
	}
	if !registrationEnabled {
		return domain.User{}, domain.ErrRegistrationDisabled
	}

	err = s.pool.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,last_login)
VALUES($1,$2,$3,now())
ON CONFLICT(username) DO UPDATE
SET email=EXCLUDED.email,display_name=EXCLUDED.display_name,last_login=now()
RETURNING id,username,email,display_name,role,enabled,session_version`, username, email, displayName).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)

	return user, err
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
