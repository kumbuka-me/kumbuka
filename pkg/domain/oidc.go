package domain

import "time"

// OIDCIdentity binds one Kumbuka account to a stable identity from an OIDC issuer.
type OIDCIdentity struct {
	// UserID identifies the user associated with OIDC identity.
	UserID int64
	// Issuer is the canonical OIDC provider identifier.
	Issuer string
	// Subject is the provider-stable account identifier.
	Subject string
	// Username is the most recently observed provider username.
	Username string
	// Email is the most recently observed provider email.
	Email string
	// DisplayName is the most recently observed provider display name.
	DisplayName string
	// LastSeenAt records the latest successful provider login.
	LastSeenAt time.Time
	// CreatedAt records the created at timestamp for OIDC identity.
	CreatedAt time.Time
}

// OIDCGroupMapping maps one external OIDC group value to a Kumbuka group.
type OIDCGroupMapping struct {
	// OIDCGroup is the external claim value mapped to a Kumbuka group.
	OIDCGroup string
	// GroupID identifies the group associated with OIDC group mapping.
	GroupID int64
	// GroupName is the group name associated with OIDC group mapping.
	GroupName string
}

// PendingOIDCIdentity is a verified but not yet accepted OIDC identity.
type PendingOIDCIdentity struct {
	// ID identifies pending OIDC identity.
	ID int64
	// Issuer is the canonical OIDC provider identifier.
	Issuer string
	// Subject is the provider-stable account identifier.
	Subject string
	// Username is the username associated with pending OIDC identity.
	Username string
	// Email is the address asserted by the provider.
	Email string
	// DisplayName is the display name associated with pending OIDC identity.
	DisplayName string
	// Status is the current status of pending OIDC identity.
	Status PendingOIDCStatus
	// FirstSeenAt records the first seen at timestamp for pending OIDC identity.
	FirstSeenAt time.Time
	// LastSeenAt records the last seen at timestamp for pending OIDC identity.
	LastSeenAt time.Time
	// SuggestedUserID identifies the suggested user associated with pending OIDC identity.
	SuggestedUserID int64
	// SuggestedUsername is the suggested username associated with pending OIDC identity.
	SuggestedUsername string
	// SuggestedDisplayName is the suggested display name associated with pending OIDC identity.
	SuggestedDisplayName string
}
