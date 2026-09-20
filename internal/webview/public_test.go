package webview

import (
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPublicLayout(t *testing.T) {
	t.Parallel()

	availableThemes := []themes.Theme{
		{Title: "Light", ColorScheme: "light"},
		{Title: "Dark", ColorScheme: "dark"},
	}
	views := &Views{
		version:      "v1.2.3",
		commit:       "abc123",
		assetVersion: "0123456789abcdef",
		themes:       availableThemes,
		runtime:      RuntimeInfo{PublicURL: "https://kumbuka.example.test"},
	}

	data, err := views.PublicData("Sign in")

	require.NoError(t, err)
	assert.Equal(t, "Sign in", data.Title)
	assert.Equal(t, themes.DefaultTheme, data.ActiveTheme)
	assert.Equal(t, themes.DefaultTheme, data.Preferences.Theme)
	assert.Equal(t, "v1.2.3", data.Version)
	assert.Equal(t, "abc123", data.Commit)
	assert.Equal(t, "0123456789abcdef", data.AssetVersion)
	assert.Equal(t, views.Runtime(), data.Runtime)
	assert.Equal(t, availableThemes, data.Themes)
	assert.Contains(t, string(data.ThemeData), `"title":"Light"`)
	assert.Contains(t, string(data.ThemeData), `"title":"Dark"`)
}
