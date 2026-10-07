package domain

import "time"

// AdminStats contains high-level object counts shown on the administration page.
type AdminStats struct {
	// Users is the number of wiki users.
	Users int64
	// Groups is the number of user groups.
	Groups int64
	// Pages is the number of active wiki pages.
	Pages int64
	// DeletedPages is the number of pages currently in the recycle bin.
	DeletedPages int64
	// Tags is the number of tags.
	Tags int64
	// Images is the number of uploaded images.
	Images int64
	// Tokens is the number of active or expired API tokens.
	Tokens int64
}

// AdminUser contains a user and the groups currently assigned to the account.
type AdminUser struct {
	// User is the wiki account.
	User User
	// HasLocalCredential reports whether the account has a local password.
	HasLocalCredential bool
	// LocalCredentialEnabled reports whether that local password can be used to sign in.
	LocalCredentialEnabled bool
	// ExternalAdminObserved reports whether an external provider supplied admin status for this account.
	ExternalAdminObserved bool
	// ExternalAdmin reports the most recently observed external administrator status.
	ExternalAdmin bool
	// Groups contains group names assigned to the user.
	Groups []string
	// OIDCIdentities contains external OIDC identities bound to the user.
	OIDCIdentities []OIDCIdentity
	// Profile describes the source and override state of mutable profile fields.
	Profile UserProfile
	// LastLogin is the most recent successful authentication time.
	LastLogin time.Time
	// HasLoggedIn reports whether LastLogin represents an actual login.
	HasLoggedIn bool
}

// ProfileSource identifies the authority that supplies an account's profile by default.
type ProfileSource string

const (
	ProfileSourceLocal        ProfileSource = "local"
	ProfileSourceOIDC         ProfileSource = "oidc"
	ProfileSourceTrustedProxy ProfileSource = "trusted-proxy"
)

// UserProfile contains local values and the latest external values used by administration.
type UserProfile struct {
	// UserID identifies the account described by this profile.
	UserID int64
	// Source identifies the authority supplying the original profile values.
	Source ProfileSource
	// Username is the effective account name, including any local override.
	Username string
	// Email is the effective email address, including any local override.
	Email string
	// DisplayName is the effective human-readable account name.
	DisplayName string
	// ProviderUsername is the latest username received from the identity provider.
	ProviderUsername string
	// ProviderEmail is the latest email address received from the identity provider.
	ProviderEmail string
	// ProviderDisplayName is the latest display name received from the identity provider.
	ProviderDisplayName string
	// UsernameOverridden reports whether a local value takes precedence over the provider username.
	UsernameOverridden bool
	// EmailOverridden reports whether a local value takes precedence over the provider email.
	EmailOverridden bool
	// DisplayNameOverridden reports whether a local value takes precedence over the provider display name.
	DisplayNameOverridden bool
	// TrustedProxyUsername is the external identity linked to this account in trusted-proxy mode.
	TrustedProxyUsername string
}

// Group describes one administratively managed user group.
type Group struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Name is the unique group name.
	Name string `json:"name"`
	// UserCount is the number of users assigned to the group.
	UserCount int64 `json:"user_count,omitempty"`
	// PageCount is the number of pages assigned to the group.
	PageCount int64 `json:"page_count,omitempty"`
}

// TagInfo describes one tag and its current page usage count.
type TagInfo struct {
	// ID is the stable identifier.
	ID int64
	// Name is the normalized tag name.
	Name string
	// PageCount is the number of pages using the tag.
	PageCount int64
}
