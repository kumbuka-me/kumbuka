package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// TrustedProxy authenticates requests using identity headers from a trusted proxy.
type TrustedProxy struct {
	// repository persists and resolves trusted-proxy users.
	repository trustedProxyRepository
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
func NewTrustedProxy(repository trustedProxyRepository, headers TrustedProxyHeaders) *TrustedProxy {
	return &TrustedProxy{repository: repository, headers: headers}
}

// Authenticate resolves the first populated trusted identity header.
func (a *TrustedProxy) Authenticate(r *http.Request) (domain.User, error) {
	username := firstHeader(r, a.headers.Username)
	if username == "" {
		return domain.User{}, ErrUnauthenticated
	}

	user, err := a.repository.TrustedProxyUser(
		r.Context(),
		username,
		firstHeader(r, a.headers.Email),
		firstHeader(r, a.headers.DisplayName),
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

	if a.headers.AdminGroup != "" {
		groups := splitHeaderValues(firstHeader(r, a.headers.Groups))
		externalAdmin := containsGroup(groups, a.headers.AdminGroup)
		if err := a.repository.SetExternalAdminStatus(r.Context(), user.ID, domain.AuthModeTrustedProxy, externalAdmin); err != nil {
			return domain.User{}, err
		}

		user.ExternalAdmin = externalAdmin
		if externalAdmin {
			user.Role = domain.UserRoleAdmin
		}
	}

	return user, nil
}

// splitHeaderValues normalizes a comma-separated trusted group header.
func splitHeaderValues(value string) []string {
	groups := make([]string, 0)
	for part := range strings.SplitSeq(value, ",") {
		if group := strings.TrimSpace(part); group != "" {
			groups = append(groups, group)
		}
	}

	return groups
}

// firstHeader returns the first non-empty configured header value.
func firstHeader(r *http.Request, headers []string) string {
	for _, header := range headers {
		if value := strings.TrimSpace(r.Header.Get(header)); value != "" {
			return value
		}
	}
	return ""
}
