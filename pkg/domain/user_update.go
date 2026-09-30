package domain

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
