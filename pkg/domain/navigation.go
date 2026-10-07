package domain

// NavigationItem describes one page or synthetic folder in the navigation tree.
type NavigationItem struct {
	// Path is the complete navigation path.
	Path string
	// Title is the page title or the raw path segment for synthetic folders.
	Title string
	// Icon is the explicitly selected icon identifier.
	Icon string
	// Page reports whether the path maps to a real wiki page.
	Page bool
}
