package domain

// PluginContentChange is one durable committed page-source event awaiting plugin delivery.
type PluginContentChange struct {
	// ID is the stable queue identifier.
	ID int64
	// PluginID identifies the active plugin selected when the page mutation committed.
	PluginID string
	// Page is the committed page snapshot observed by plugins.
	Page Page
	// PreviousMarkdown is the canonical source stored before the mutation.
	PreviousMarkdown string
	// Markdown is the canonical source stored by the mutation.
	Markdown string
	// ActorID identifies the user that committed the mutation.
	ActorID int64
	// Attempts counts claimed delivery attempts, including the current attempt.
	Attempts int
}
