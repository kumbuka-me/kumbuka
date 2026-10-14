package authentication

import (
	"context"
	"errors"

	"github.com/containeroo/appkit/auth/provisioning"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// oidcRepository contains persistence and settings reads required for OIDC login resolution.
type oidcRepository interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	OIDCUser(context.Context, string, string) (domain.User, error)
	ResolveOIDCLogin(context.Context, string, string, string, string, string, bool) (domain.User, error)
}

// OIDC coordinates verified OIDC identity login policy with atomic persistence.
type OIDC struct {
	// resolver coordinates generic external-identity provisioning with Kumbuka storage.
	resolver provisioning.Resolver[domain.User]
}

// NewOIDC constructs the OIDC login application service.
func NewOIDC(repository oidcRepository, registrationOverride *bool) *OIDC {
	return &OIDC{
		resolver: provisioning.Resolver[domain.User]{
			Registration: provisioning.RegistrationPolicy{
				Override: registrationOverride,
				Load: func(ctx context.Context) (bool, error) {
					settings, err := repository.ApplicationSettings(ctx)
					return settings.AllowUserRegistration, err
				},
			},
			IsMissing: func(err error) bool { return errors.Is(err, domain.ErrNotFound) },
			Lookup: func(ctx context.Context, identity provisioning.Identity) (domain.User, error) {
				return repository.OIDCUser(ctx, identity.Issuer, identity.Subject)
			},
			Resolve: func(ctx context.Context, identity provisioning.Identity, registrationEnabled bool) (domain.User, error) {
				return repository.ResolveOIDCLogin(ctx, identity.Issuer, identity.Subject, identity.Username, identity.Email, identity.DisplayName, registrationEnabled)
			},
		},
	}
}

// Login normalizes a verified provider identity, resolves registration policy, and delegates the atomic account mutation.
func (s *OIDC) Login(ctx context.Context, issuer, subject, username, email, displayName string) (domain.User, error) {
	user, err := s.resolver.Login(ctx, provisioning.Identity{
		Issuer: issuer, Subject: subject, Username: username, Email: email, DisplayName: displayName,
	})
	if errors.Is(err, provisioning.ErrIncompleteIdentity) {
		return domain.User{}, domain.NewValidationError("identity", "The identity provider must supply issuer, subject, and username.")
	}
	return user, err
}
