package domain

// UserAccountUpdate contains one atomic account mutation. An empty password hash preserves the credential.
type UserAccountUpdate struct {
	// UserID identifies the user associated with user account update.
	UserID int64
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
