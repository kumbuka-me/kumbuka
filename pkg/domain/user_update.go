package domain

import "strings"

// UserAccountUpdate contains one atomic account mutation. Nil profile fields and an empty password hash preserve existing values.
type UserAccountUpdate struct {
	// UserID identifies the account to update.
	UserID int64
	// Username optionally replaces the administrator-managed username.
	Username *string
	// Email optionally replaces the administrator-managed email address. A pointer to an empty string clears the address.
	Email *string
	// DisplayName optionally replaces the administrator-managed display name.
	DisplayName *string
	// RevertUsername restores the latest provider username and clears its override.
	RevertUsername bool
	// RevertEmail restores the latest provider email and clears its override.
	RevertEmail bool
	// RevertDisplayName restores the latest provider display name and clears its override.
	RevertDisplayName bool
	// Role replaces the account role.
	Role UserRole
	// Enabled determines whether the account may authenticate.
	Enabled bool
	// GroupIDs replaces the complete set of account group memberships.
	GroupIDs []int64
	// LocalCredentialEnabled optionally changes whether the local recovery credential may authenticate.
	LocalCredentialEnabled *bool
	// PasswordHash replaces the local password hash when nonempty.
	PasswordHash string
}

// UpdatedProfile resolves the effective account fields and override flags against the current provider values.
func (u UserAccountUpdate) UpdatedProfile(profile UserProfile) UserProfile {
	providerManaged := profile.Source != ProfileSourceLocal
	profile.Username, profile.UsernameOverridden = updatedProfileField(
		profile.Username, profile.UsernameOverridden, profile.ProviderUsername,
		u.Username, u.RevertUsername, providerManaged,
	)
	profile.Email, profile.EmailOverridden = updatedProfileField(
		profile.Email, profile.EmailOverridden, profile.ProviderEmail,
		u.Email, u.RevertEmail, providerManaged,
	)
	profile.DisplayName, profile.DisplayNameOverridden = updatedProfileField(
		profile.DisplayName, profile.DisplayNameOverridden, profile.ProviderDisplayName,
		u.DisplayName, u.RevertDisplayName, providerManaged,
	)
	return profile
}

// updatedProfileField gives a provider revert precedence over an explicit replacement.
func updatedProfileField(current string, overridden bool, provider string, replacement *string, revert, providerManaged bool) (string, bool) {
	switch {
	case revert:
		return provider, false
	case replacement != nil:
		return strings.TrimSpace(*replacement), providerManaged
	default:
		return current, overridden
	}
}
