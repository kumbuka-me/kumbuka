package domain

import "time"

// PageAccess is the effective nearest path-rule decision for one user.
type PageAccess struct {
	// Restricted reports whether a path-specific access rule applies.
	Restricted bool
	// CanView reports whether the actor may read the page.
	CanView bool
	// CanEdit reports whether the actor may change the page.
	CanEdit bool
}

// PageAccessRule grants one group view or edit access at a path.
type PageAccessRule struct {
	// ID identifies page access rule.
	ID int64
	// Path is the path associated with page access rule.
	Path string
	// GroupID identifies the group associated with page access rule.
	GroupID int64
	// GroupName is the group name associated with page access rule.
	GroupName string
	// Access is the permission granted to the group.
	Access PageAccessLevel
	// CreatedAt records the created at timestamp for page access rule.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for page access rule.
	UpdatedAt time.Time
}
