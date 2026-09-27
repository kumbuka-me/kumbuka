package authentication

import (
	"context"
	"errors"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// oidcRepository contains persistence and settings reads required for OIDC login resolution.
type oidcRepository interface {
	registrationSettingsRepository
	OIDCUser(context.Context, string, string) (domain.User, error)
	ResolveOIDCLogin(context.Context, string, string, string, string, string, bool) (domain.User, error)
}

// OIDC coordinates verified OIDC identity login policy with atomic persistence.
type OIDC struct {
	// repository provides identity lookup and atomic login persistence.
	repository oidcRepository
	// registration resolves whether unknown external identities may create accounts.
	registration registrationPolicy
}

// NewOIDC constructs the OIDC login application service.
func NewOIDC(repository oidcRepository, registrationOverride *bool) *OIDC {
	return &OIDC{
		repository:   repository,
		registration: newRegistrationPolicy(repository, registrationOverride),
	}
}

// Login normalizes a verified provider identity, resolves registration policy, and delegates the atomic account mutation.
func (s *OIDC) Login(ctx context.Context, issuer, subject, username, email, displayName string) (domain.User, error) {
	issuer = strings.TrimSpace(issuer)
	subject = strings.TrimSpace(subject)
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if issuer == "" || subject == "" || username == "" {
		return domain.User{}, domain.NewValidationError("identity", "The identity provider must supply issuer, subject, and username.")
	}

	registrationEnabled := false
	if _, err := s.repository.OIDCUser(ctx, issuer, subject); err != nil {
		if !errors.Is(err, domain.ErrNotFound) {
			return domain.User{}, err
		}
		registrationEnabled, err = s.registration.enabled(ctx)
		if err != nil {
			return domain.User{}, err
		}
	}
	return s.repository.ResolveOIDCLogin(ctx, issuer, subject, username, email, displayName, registrationEnabled)
}
