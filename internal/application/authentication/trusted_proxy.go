package authentication

import (
	"context"
	"errors"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// trustedProxyRepository contains persistence required to refresh or create trusted-proxy users.
type trustedProxyRepository interface {
	registrationSettingsRepository
	RefreshTrustedProxyUser(context.Context, string, string, string) (domain.User, error)
	CreateTrustedProxyUser(context.Context, string, string, string) (domain.User, error)
}

// TrustedProxy owns trusted-proxy identity normalization and registration policy.
type TrustedProxy struct {
	// repository refreshes existing accounts and creates permitted external identities.
	repository trustedProxyRepository
	// registration resolves whether unknown external identities may create accounts.
	registration registrationPolicy
}

// NewTrustedProxy constructs the trusted-proxy login application service.
func NewTrustedProxy(repository trustedProxyRepository, registrationOverride *bool) *TrustedProxy {
	return &TrustedProxy{
		repository:   repository,
		registration: newRegistrationPolicy(repository, registrationOverride),
	}
}

// Login refreshes an existing trusted-proxy account or creates one when registration is enabled.
func (s *TrustedProxy) Login(ctx context.Context, username, email, displayName string) (domain.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if username == "" {
		return domain.User{}, domain.NewValidationError("identity", "The trusted proxy must supply a username.")
	}

	user, err := s.repository.RefreshTrustedProxyUser(ctx, username, email, displayName)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.User{}, err
	}

	enabled, err := s.registration.enabled(ctx)
	if err != nil {
		return domain.User{}, err
	}
	if !enabled {
		return domain.User{}, domain.ErrRegistrationDisabled
	}

	return s.repository.CreateTrustedProxyUser(ctx, username, email, displayName)
}
