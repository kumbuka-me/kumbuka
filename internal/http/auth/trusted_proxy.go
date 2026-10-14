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

// TrustedProxyHeaders contains ordered header candidates and external administrator group mapping.
type TrustedProxyHeaders struct {
	// Username lists trusted header names checked for the external username.
	Username []string
	// Email lists trusted header names checked for the external email address.
	Email []string
	// DisplayName lists trusted header names checked for the external display name.
	DisplayName []string
	// Groups lists trusted header names checked for comma-separated external groups.
	Groups []string
	// AdminGroup names the external group that grants administrator access.
	AdminGroup string
}

// NewTrustedProxy creates an authenticator that requires an explicitly trusted ingress.
func NewTrustedProxy(
	login trustedProxyLoginService,
	headers TrustedProxyHeaders,
	trust appkitproxy.TrustFunc,
) *TrustedProxy {
	return &TrustedProxy{authenticator: appkitproxy.Authenticator[domain.User]{
		Trust: trust,
		Resolve: func(ctx context.Context, identity appkitauth.ExternalIdentity) (domain.User, error) {
			adminObserved := headers.AdminGroup != ""
			externalAdmin := adminObserved && containsGroup(identity.Groups, headers.AdminGroup)
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
		},
		Issuer:             "kumbuka-proxy",
		UsernameHeaders:    headers.Username,
		EmailHeaders:       headers.Email,
		DisplayNameHeaders: headers.DisplayName,
		GroupHeaders:       headers.Groups,
	}}
}

// Authenticate authenticates only identities asserted by an allowed immediate network peer.
func (a *TrustedProxy) Authenticate(r *http.Request) (domain.User, error) {
	if a.authenticator.Trust == nil {
		return domain.User{}, ErrUnauthenticated
	}
	return a.authenticator.Authenticate(r)
}
