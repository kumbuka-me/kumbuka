package domain

import "time"

// APIToken contains non-secret metadata for one personal access token.
type APIToken struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Name is the user-supplied token label.
	Name string `json:"name"`
	// UserID is the account authenticated by the token.
	UserID int64 `json:"user_id"`
	// Username is the login name authenticated by the token.
	Username string `json:"username"`
	// CreatedBy is the account that issued the token.
	CreatedBy int64 `json:"created_by"`
	// Creator is the display name of the account that issued the token.
	Creator string `json:"creator"`
	// CreatedAt is the issuance timestamp.
	CreatedAt time.Time `json:"created_at"`
	// LastUsed is the most recent successful authentication timestamp.
	LastUsed *time.Time `json:"last_used,omitempty"`
	// ExpiresAt is the optional expiration timestamp.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IssuedToken contains token metadata and the one-time plaintext secret.
type IssuedToken struct {
	// Token contains the non-secret token metadata.
	Token APIToken `json:"token"`
	// Secret is the plaintext token shown only immediately after creation.
	Secret string `json:"secret"`
}
