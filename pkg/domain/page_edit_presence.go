package domain

import "time"

// PageEditorPresence describes one user who recently confirmed that they are editing a page.
type PageEditorPresence struct {
	// UserID identifies the editor without exposing private account fields.
	UserID int64 `json:"user_id"`
	// Name is the editor display name shown to other authenticated page viewers.
	Name string `json:"name"`
	// UpdatedAt records the most recent editor heartbeat.
	UpdatedAt time.Time `json:"updated_at"`
}
