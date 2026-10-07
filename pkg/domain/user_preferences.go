package domain

// UserPreferences contains presentation preferences for one wiki user.
type UserPreferences struct {
	// Locale is the optional BCP 47 interface language; empty follows the browser language.
	Locale string
	// Theme is the filename-derived title of the user's selected theme.
	Theme string
	// ShowPageContents controls whether wiki pages render a heading table of contents.
	ShowPageContents bool
	// NavigationStyle controls the desktop navigation layout.
	NavigationStyle NavigationStyle
	// NavigationDensity controls vertical spacing in the page tree.
	NavigationDensity NavigationDensity
	// TypographySize overrides the application content size; empty inherits the administrator default.
	TypographySize TypographySize
	// SidebarWidth is the desktop sidebar width in CSS pixels.
	SidebarWidth int
	// ShowNavigationGuides controls tree indentation guide lines.
	ShowNavigationGuides bool
	// RememberNavigationState persists expanded and collapsed navigation folders.
	RememberNavigationState bool
	// HiddenPluginWidgets contains plugin/module keys hidden by this user.
	HiddenPluginWidgets []string
	// ShowNavigationPageCounts displays descendant page counts for folders.
	ShowNavigationPageCounts bool
	// ExpandedNavigation contains folder slugs the user explicitly left expanded.
	ExpandedNavigation []string
}
