package domain

// MovePageOptions controls safe page-tree refactoring.
type MovePageOptions struct {
	// MoveChildren moves every descendant along with the selected page.
	MoveChildren bool
	// UpdateIncomingLinks rewrites wiki links that target moved pages.
	UpdateIncomingLinks bool
}
