package postgres

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// CreateToken generates and stores a personal access token for the selected user.
func (s *Store) CreateToken(
	ctx context.Context,
	name string,
	userID, createdBy int64,
	expiresAt *time.Time,
) (domain.IssuedToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.IssuedToken{}, domain.NewValidationError("name", "A token name is required.")
	}

	user, err := s.User(ctx, userID)
	if err != nil {
		return domain.IssuedToken{}, err
	}

	creator, err := s.User(ctx, createdBy)
	if err != nil {
		return domain.IssuedToken{}, err
	}

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return domain.IssuedToken{}, err
	}

	secret := "kumbuka_pat_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	hash := sha256.Sum256([]byte(secret))
	hashString := hex.EncodeToString(hash[:])

	var token domain.APIToken
	err = s.pool.QueryRow(ctx, `
INSERT INTO api_tokens(name,token_hash,user_id,created_by,expires_at)
VALUES($1,$2,$3,$4,$5)
RETURNING id,name,user_id,coalesce(created_by,0),created_at`, name, hashString, userID, createdBy, expiresAt).Scan(
		&token.ID,
		&token.Name,
		&token.UserID,
		&token.CreatedBy,
		&token.CreatedAt,
	)
	if err != nil {
		return domain.IssuedToken{}, err
	}

	token.ExpiresAt = expiresAt

	token.Username = user.Username
	token.Creator = cmp.Or(creator.DisplayName, creator.Username)

	return domain.IssuedToken{Token: token, Secret: secret}, nil
}

// UserTokens returns token metadata for one authenticated account.
func (s *Store) UserTokens(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	return s.tokens(ctx, `WHERE coalesce(t.user_id,t.created_by)=$1`, userID)
}

// Tokens returns all token metadata for administrators.
func (s *Store) Tokens(ctx context.Context) ([]domain.APIToken, error) {
	return s.tokens(ctx, "")
}

// tokens queries token metadata with an optional WHERE clause and arguments.
func (s *Store) tokens(ctx context.Context, where string, args ...any) ([]domain.APIToken, error) {
	rows, err := s.pool.Query(ctx, `
SELECT
  t.id,
  t.name,
  coalesce(t.user_id,t.created_by,0),
  coalesce(subject.username,''),
  coalesce(t.created_by,0),
  coalesce(creator.display_name,creator.username,''),
  t.created_at,
  t.last_used,
  t.expires_at
FROM api_tokens t
LEFT JOIN users subject ON subject.id=coalesce(t.user_id,t.created_by)
LEFT JOIN users creator ON creator.id=t.created_by
`+where+`
ORDER BY t.created_at DESC,t.id DESC`, args...)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	var tokens []domain.APIToken

	for rows.Next() {
		var token domain.APIToken
		var lastUsed pgtype.Timestamptz
		var expiresAt pgtype.Timestamptz
		if err := rows.Scan(
			&token.ID,
			&token.Name,
			&token.UserID,
			&token.Username,
			&token.CreatedBy,
			&token.Creator,
			&token.CreatedAt,
			&lastUsed,
			&expiresAt,
		); err != nil {
			return nil, err
		}

		if lastUsed.Valid {
			value := lastUsed.Time
			token.LastUsed = &value
		}
		if expiresAt.Valid {
			value := expiresAt.Time
			token.ExpiresAt = &value
		}

		tokens = append(tokens, token)
	}

	return tokens, rows.Err()
}

// DeleteUserToken removes a token owned by the supplied account.
func (s *Store) DeleteUserToken(ctx context.Context, id, userID int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM api_tokens
WHERE id=$1 AND coalesce(user_id,created_by)=$2`, id, userID)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}

// DeleteToken removes a token by identifier for administrators.
func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `
DELETE FROM api_tokens
WHERE id=$1`, id)
	if err == nil && tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}

	return err
}
