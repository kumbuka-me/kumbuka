package tokens

import (
	"context"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// tokenRepository contains personal access token operations.
type tokenRepository interface {
	Tokens(context.Context) ([]domain.APIToken, error)
	UserTokens(context.Context, int64) ([]domain.APIToken, error)
	CreateToken(context.Context, string, int64, int64, *time.Time) (domain.IssuedToken, error)
	DeleteUserToken(context.Context, int64, int64) error
	DeleteToken(context.Context, int64) error
}

// Tokens exposes personal access token use cases.
type Tokens struct {
	// repository provides the persistence operations required by tokens.
	repository tokenRepository
}

// NewTokens constructs the personal access token service.
func NewTokens(repository tokenRepository) *Tokens { return &Tokens{repository: repository} }

// Tokens returns all personal access tokens for administration.
func (s *Tokens) Tokens(ctx context.Context) ([]domain.APIToken, error) {
	return s.repository.Tokens(ctx)
}

// UserTokens returns personal access tokens owned by a user.
func (s *Tokens) UserTokens(ctx context.Context, userID int64) ([]domain.APIToken, error) {
	return s.repository.UserTokens(ctx, userID)
}

// CreateToken issues a new personal access token.
func (s *Tokens) CreateToken(
	ctx context.Context,
	name string,
	userID, createdBy int64,
	expiresAt *time.Time,
) (domain.IssuedToken, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.IssuedToken{}, domain.NewValidationError("name", "A token name is required.")
	}
	return s.repository.CreateToken(ctx, name, userID, createdBy, expiresAt)
}

// DeleteUserToken removes a personal access token owned by a user.
func (s *Tokens) DeleteUserToken(ctx context.Context, id, userID int64) error {
	return s.repository.DeleteUserToken(ctx, id, userID)
}

// DeleteToken removes any personal access token by identifier.
func (s *Tokens) DeleteToken(ctx context.Context, id int64) error {
	return s.repository.DeleteToken(ctx, id)
}
