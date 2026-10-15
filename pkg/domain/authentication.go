package domain

// AuthMode identifies a supported browser authentication mode.
type AuthMode string

const (
	// AuthModeNone authenticates every request as the local administrator.
	AuthModeNone AuthMode = "none"
	// AuthModeLocal authenticates browser requests with Kumbuka-managed credentials.
	AuthModeLocal AuthMode = "local"
	// AuthModeProxy authenticates users from trusted proxy headers.
	AuthModeProxy AuthMode = "proxy"
	// AuthModeOIDC authenticates users through an OIDC provider.
	AuthModeOIDC AuthMode = "oidc"
)

// IsExternalAuthMode reports whether mode relies on an external identity boundary.
func IsExternalAuthMode(mode AuthMode) bool {
	return mode == AuthModeOIDC || mode == AuthModeProxy
}
