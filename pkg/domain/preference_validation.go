package domain

// ValidNavigationStyle reports whether value is a supported desktop navigation layout.
func ValidNavigationStyle(value NavigationStyle) bool {
	switch value {
	case NavigationStyleSidebar, NavigationStyleTopbar, NavigationStyleTree:
		return true
	default:
		return false
	}
}

// ValidNavigationDensity reports whether value is a supported navigation density.
func ValidNavigationDensity(value NavigationDensity) bool {
	return value == NavigationDensityComfortable || value == NavigationDensityCompact
}

// ValidRobotsPolicy reports whether value is a supported robots.txt policy.
func ValidRobotsPolicy(value RobotsPolicy) bool {
	switch value {
	case RobotsPolicyAllow, RobotsPolicyDisallow, RobotsPolicyNone:
		return true
	default:
		return false
	}
}

// ValidTypographySize reports whether value is a supported content typography preset.
func ValidTypographySize(value TypographySize) bool {
	switch value {
	case TypographySizeCompact, TypographySizeStandard, TypographySizeLarge:
		return true
	default:
		return false
	}
}

// ValidSidebarWidth reports whether width is inside the supported desktop range.
func ValidSidebarWidth(width int) bool {
	return width >= MinSidebarWidth && width <= MaxSidebarWidth
}

// ValidReviewIntervalDays reports whether days is a supported review interval.
func ValidReviewIntervalDays(days int) bool {
	return days >= 0 && days <= 3650
}

// DefaultUserPreferences returns the presentation defaults used before a user saves preferences.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		ShowPageContents:         true,
		NavigationStyle:          NavigationStyleSidebar,
		NavigationDensity:        NavigationDensityComfortable,
		SidebarWidth:             DefaultSidebarWidth,
		ShowNavigationGuides:     true,
		RememberNavigationState:  true,
		ShowNavigationPageCounts: false,
		ExpandedNavigation:       []string{},
		HiddenPluginWidgets:      []string{},
	}
}
