package domain

// UserAccountUpdate contains one atomic account mutation. An empty password hash preserves the credential.
type UserAccountUpdate struct {
	UserID                 int64
	Role                   string
	Enabled                bool
	GroupIDs               []int64
	LocalCredentialEnabled *bool
	PasswordHash           string
}
