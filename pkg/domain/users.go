package domain

// User represents an authenticated wiki account.
type User struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Username is the unique login name.
	Username string `json:"username"`
	// Email is the account email address.
	Email string `json:"email"`
	// DisplayName is the human-readable account name.
	DisplayName string `json:"display_name"`
	// Role controls the account authorization level.
	Role UserRole `json:"role"`
	// Enabled controls authentication through every method.
	Enabled bool `json:"-"`
	// ExternalAdmin is an authentication-time administrator elevation.
	ExternalAdmin bool `json:"-"`
	// SessionVersion invalidates previously issued external browser sessions when incremented.
	SessionVersion int64 `json:"-"`
}

// IsAdministrator reports whether the user has effective administrator access.
func (u User) IsAdministrator() bool {
	return u.Role == UserRoleAdmin || u.ExternalAdmin
}

// CanEditContent reports whether the account role permits editing before page-specific access rules.
func (u User) CanEditContent() bool {
	return u.IsAdministrator() || u.Role == UserRoleEditor
}

// ValidUserRole reports whether value is a supported account role.
func ValidUserRole(value UserRole) bool {
	switch value {
	case UserRoleAdmin, UserRoleEditor, UserRoleViewer:
		return true
	default:
		return false
	}
}
