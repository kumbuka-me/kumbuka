package postgres

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestDefaultUserPreferences(t *testing.T) {
	t.Parallel()

	preferences := domain.DefaultUserPreferences()

	assert.True(t, preferences.ShowPageContents)
	assert.Equal(t, domain.NavigationStyleSidebar, preferences.NavigationStyle)
	assert.Equal(t, domain.NavigationDensityComfortable, preferences.NavigationDensity)
	assert.Empty(t, preferences.TypographySize)
	assert.Equal(t, domain.DefaultSidebarWidth, preferences.SidebarWidth)
	assert.True(t, preferences.ShowNavigationGuides)
	assert.True(t, preferences.RememberNavigationState)
	assert.False(t, preferences.ShowNavigationPageCounts)
	assert.Empty(t, preferences.ExpandedNavigation)
	assert.Empty(t, preferences.HiddenPluginWidgets)
}

func TestNormalizeNavigationPaths(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"applications", "applications/identity"}, normalizeNavigationPaths([]string{
		" /applications/ ",
		"applications",
		"",
		"applications/identity",
	}))
}
