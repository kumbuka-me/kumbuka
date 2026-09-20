package postgres

import (
	"cmp"
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// SetupRequired reports whether Kumbuka still has no user accounts.
func (s *Store) SetupRequired(ctx context.Context) (bool, error) {
	var required bool
	err := s.pool.QueryRow(ctx, `
SELECT NOT EXISTS(SELECT 1 FROM users)`).Scan(&required)

	return required, err
}

// CreateInitialLocalAdministrator creates the first account and switches fresh installs to local authentication.
func (s *Store) CreateInitialLocalAdministrator(
	ctx context.Context,
	username, email, displayName, passwordHash string,
) (domain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if username == "" || passwordHash == "" {
		return domain.User{}, domain.NewValidationError("username", "A username and password are required.")
	}

	displayName = cmp.Or(displayName, username)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	// Lock the singleton settings row so only one concurrent setup can win.
	// The presence of users is the setup invariant; a stale auth_mode must not
	// make an otherwise empty installation impossible to bootstrap.
	var singleton bool
	if err := tx.QueryRow(ctx, `
SELECT singleton
FROM application_settings
WHERE singleton=true FOR UPDATE`).Scan(&singleton); err != nil {
		return domain.User{}, err
	}

	var exists bool
	if err := tx.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists); err != nil {
		return domain.User{}, err
	}
	if exists {
		return domain.User{}, domain.ErrAlreadyExists
	}

	var user domain.User
	if err := tx.QueryRow(ctx, `
INSERT INTO users(username,email,display_name,role,last_login)
VALUES($1,$2,$3,'admin',now())
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
INSERT INTO local_credentials(user_id,password_hash)
VALUES($1,$2)`, user.ID, passwordHash); err != nil {
		return domain.User{}, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE application_settings
SET auth_mode='local',allow_user_registration=false,updated_at=now()
WHERE singleton=true`); err != nil {
		return domain.User{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, err
	}

	return user, nil
}

// LocalCredential returns one local-login account and its password hash.
func (s *Store) LocalCredential(ctx context.Context, username string) (user domain.User, passwordHash string, err error) {
	err = s.pool.QueryRow(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version,c.password_hash
FROM local_credentials c
JOIN users u ON u.id=c.user_id
WHERE u.username=$1 AND u.enabled AND c.enabled`, strings.TrimSpace(username)).Scan(
		&user.ID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Role,
		&user.Enabled,
		&user.SessionVersion,
		&passwordHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, "", domain.ErrNotFound
	}

	return user, passwordHash, err
}

// HasLocalCredential reports whether one Kumbuka user has a local password.
func (s *Store) HasLocalCredential(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(SELECT 1 FROM local_credentials WHERE user_id=$1)`, userID).Scan(&exists)

	return exists, err
}

// HasLocalAdministratorCredential reports whether local mode has an administrator who can sign in.
func (s *Store) HasLocalAdministratorCredential(ctx context.Context) (bool, error) {
	var exists bool
	err := s.pool.QueryRow(ctx, `
SELECT EXISTS(
  SELECT 1
  FROM local_credentials c
  JOIN users u ON u.id=c.user_id
  WHERE u.role='admin' AND u.enabled AND c.enabled
)`).Scan(&exists)

	return exists, err
}

// SetLocalCredential creates or replaces a user's local password hash.
func (s *Store) SetLocalCredential(ctx context.Context, userID int64, passwordHash string) error {
	if userID <= 0 || passwordHash == "" {
		return domain.NewValidationError("user_id", "A user and password are required.")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if err := setLocalCredential(ctx, tx, userID, passwordHash); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// setLocalCredential replaces a credential and revokes its sessions within the caller's transaction.
func setLocalCredential(ctx context.Context, tx pgx.Tx, userID int64, passwordHash string) error {
	tag, err := tx.Exec(ctx, `
INSERT INTO local_credentials(user_id,password_hash,enabled,updated_at)
SELECT id,$2,true,now() FROM users WHERE id=$1
ON CONFLICT(user_id) DO UPDATE
SET password_hash=EXCLUDED.password_hash,enabled=true,updated_at=now()`, userID, passwordHash)
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

	return nil
}

// CreateLocalSession persists a hashed local-login session and records login time.
func (s *Store) CreateLocalSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}

	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
DELETE FROM local_sessions
WHERE expires_at<=now()`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO local_sessions(token_hash,user_id,expires_at)
VALUES($1,$2,$3)`, tokenHash, userID, expiresAt); err != nil {
		return err
	}

	tag, err := tx.Exec(ctx, `
UPDATE users
SET last_login=now()
WHERE id=$1`, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return tx.Commit(ctx)
}

// LocalUserBySession resolves a non-expired local-login session.
func (s *Store) LocalUserBySession(ctx context.Context, tokenHash string) (domain.User, error) {
	var user domain.User
	err := s.pool.QueryRow(ctx, `
SELECT u.id,u.username,u.email,u.display_name,u.role,u.enabled,u.session_version
FROM local_sessions s
JOIN users u ON u.id=s.user_id
JOIN local_credentials c ON c.user_id=u.id
WHERE s.token_hash=$1 AND s.expires_at>now() AND u.enabled AND c.enabled`, tokenHash).Scan(&user.ID, &user.Username, &user.Email, &user.DisplayName, &user.Role, &user.Enabled, &user.SessionVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrNotFound
	}

	return user, err
}

// DeleteLocalSession revokes one local-login session when it exists.
func (s *Store) DeleteLocalSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `
DELETE FROM local_sessions
WHERE token_hash=$1`, tokenHash)
	return err
}
