package auth

import (
	"context"
	"errors"
	"net/http"

	appkitauth "github.com/containeroo/appkit/auth"
	appkitproxy "github.com/containeroo/appkit/auth/proxy"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// TrustedProxy authenticates verified proxy identity headers using Kumbuka's local account policy.
type TrustedProxy struct {
	// authenticator verifies the upstream and extracts external identity claims.
	authenticator appkitproxy.Authenticator[domain.User]
}

// NewTrustedProxy creates an authenticator that requires an explicitly trusted ingress.
func NewTrustedProxy(
	login trustedProxyLoginService,
	cfg appkitproxy.Config,
	adminGroup string,
) (*TrustedProxy, error) {
	provider, err := appkitproxy.New(cfg, "kumbuka-proxy", func(ctx context.Context, identity appkitauth.ExternalIdentity) (domain.User, error) {
		adminObserved := adminGroup != ""
		externalAdmin := adminObserved && containsGroup(identity.Groups, adminGroup)
		user, err := login.Login(ctx, identity.Username, identity.Email, identity.DisplayName, adminObserved, externalAdmin)
		if errors.Is(err, domain.ErrRegistrationDisabled) {
			return domain.User{}, ErrRegistrationDisabled
		}
		if err != nil {
			return domain.User{}, err
		}
		if !user.Enabled {
			return domain.User{}, ErrInvalidCredentials
		}
		if adminObserved {
			user.ExternalAdmin = externalAdmin
			if externalAdmin {
				user.Role = domain.UserRoleAdmin
			}
		}
		return user, nil
	})
	if err != nil {
		return nil, err
	}
	return &TrustedProxy{authenticator: provider}, nil
}

// Authenticate authenticates only identities asserted by an allowed immediate network peer.
func (a *TrustedProxy) Authenticate(r *http.Request) (domain.User, error) {
	return a.authenticator.Authenticate(r)
}
