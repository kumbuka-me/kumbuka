package domain

// PageLink describes one wiki link recorded for a source page.
type PageLink struct {
	// TargetID is the stable identity of the resolved target, or zero when missing.
	TargetID int64
	// ResolvedSlug is the current slug of the resolved target, or empty when missing.
	ResolvedSlug string
	// TargetSlug is the target slug associated with page link.
	TargetSlug string
	// TargetTitle is the target title associated with page link.
	TargetTitle string
	// Exists reports whether the target resolved to a current page or alias.
	Exists bool
}
