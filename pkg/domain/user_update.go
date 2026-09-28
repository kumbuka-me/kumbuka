package domain

// UserAccountUpdate contains one atomic account mutation. Nil profile fields and an empty password hash preserve existing values.
type UserAccountUpdate struct {
	// UserID identifies the user associated with user account update.
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
	// Role is the role associated with user account update.
	Role UserRole
	// Enabled reports whether enabled applies to user account update.
	Enabled bool
	// GroupIDs contains the group i ds associated with user account update.
	GroupIDs []int64
	// LocalCredentialEnabled stores the local credential enabled value used by user account update.
	LocalCredentialEnabled *bool
	// PasswordHash stores the password hash value used by user account update.
	PasswordHash string
}
