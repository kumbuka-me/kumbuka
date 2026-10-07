package domain

import "time"

// AuditEvent describes one administratively visible application action.
type AuditEvent struct {
	// ID identifies audit event.
	ID int64
	// Actor is the display name of the user who performed the action.
	Actor string
	// Action identifies the operation that was performed.
	Action string
	// ObjectType is the object type associated with audit event.
	ObjectType string
	// ObjectKey identifies the affected domain object.
	ObjectKey string
	// Detail is a human-readable description of the change.
	Detail string
	// CreatedAt records the created at timestamp for audit event.
	CreatedAt time.Time
}
