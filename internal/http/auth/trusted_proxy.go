package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/utils"
)

// TrustedProxy authenticates requests using identity headers from a trusted proxy.
type TrustedProxy struct {
	// login resolves trusted identities through application registration policy.
	login trustedProxyLoginService
	// headers maps trusted proxy headers to Kumbuka identity fields.
	headers TrustedProxyHeaders
}

// TrustedProxyHeaders contains ordered header candidates for each external identity field.
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

// NewTrustedProxy creates a trusted-proxy authenticator.
func NewTrustedProxy(
	login trustedProxyLoginService,
	headers TrustedProxyHeaders,
) *TrustedProxy {
	return &TrustedProxy{
		login:   login,
		headers: headers,
	}
}

// Authenticate resolves trusted identity headers in configured fallback order.
func (a *TrustedProxy) Authenticate(r *http.Request) (domain.User, error) {
	username := firstConfiguredHeaderValue(r, a.headers.Username)
	if username == "" {
		return domain.User{}, ErrUnauthenticated
	}

	adminObserved := a.headers.AdminGroup != ""
	externalAdmin := false
	if adminObserved {
		groups := utils.SplitCommaSeparatedValues(firstConfiguredHeaderValue(r, a.headers.Groups))
		externalAdmin = containsGroup(groups, a.headers.AdminGroup)
	}

	user, err := a.login.Login(
		r.Context(),
		username,
		firstConfiguredHeaderValue(r, a.headers.Email),
		firstConfiguredHeaderValue(r, a.headers.DisplayName),
		adminObserved,
		externalAdmin,
	)
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
}

// firstConfiguredHeaderValue returns the first non-empty configured header.
// Example: ["X-User", "X-Remote-User"] uses X-Remote-User only when X-User is blank.
func firstConfiguredHeaderValue(r *http.Request, headers []string) string {
	for _, header := range headers {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}
