package themes

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTheme = `color_scheme = "dark"

[colors]
background = "#000001"
surface = "#000002"
surface_elevated = "#000003"
surface_hover = "#000004"
text = "#ffffff"
text_secondary = "#eeeeee"
text_tertiary = "#dddddd"
muted = "#aaaaaa"
border = "#222222"
border_strong = "#333333"
border_subtle = "#111111"
accent = "#123456"
accent_secondary = "#234567"
accent_soft = "#345678"
success = "#456789"
warning = "#56789a"
error = "#6789ab"
danger = "#789abc"
selection_text = "#ffffff"
selection_background = "#123456"
`

func TestLoadEmbeddedThemeCatalog(t *testing.T) {
	t.Parallel()

	available, err := Load(Files, "")

	require.NoError(t, err)
	assert.Len(t, available, 16)

	tests := []struct {
		name   string
		scheme ColorScheme
	}{
		{name: "Catppuccin Frappe", scheme: ColorSchemeDark},
		{name: "Catppuccin Latte", scheme: ColorSchemeLight},
		{name: "Catppuccin Macchiato", scheme: ColorSchemeDark},
		{name: "Catppuccin Mocha", scheme: ColorSchemeDark},
		{name: "Dark", scheme: ColorSchemeDark},
		{name: "Dracula", scheme: ColorSchemeDark},
		{name: "Gruvbox Dark", scheme: ColorSchemeDark},
		{name: "Gruvbox Light", scheme: ColorSchemeLight},
		{name: "Light", scheme: ColorSchemeLight},
		{name: "Nord", scheme: ColorSchemeDark},
		{name: "One Dark", scheme: ColorSchemeDark},
		{name: "Rose Pine", scheme: ColorSchemeDark},
		{name: "Rose Pine Dawn", scheme: ColorSchemeLight},
		{name: "Solarized Dark", scheme: ColorSchemeDark},
		{name: "Solarized Light", scheme: ColorSchemeLight},
		{name: "Tokyo Night", scheme: ColorSchemeDark},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			theme, ok := Find(available, test.name)

			require.True(t, ok, "embedded theme is missing")
			assert.Equal(t, test.scheme, theme.ColorScheme)
		})
	}
}

func TestLoadUsesProvidedThemeSource(t *testing.T) {
	t.Parallel()

	available, err := Load(fstest.MapFS{
		"Fixture.toml": &fstest.MapFile{Data: []byte(testTheme)},
	}, "")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Equal(t, "Fixture", available[0].Title)
	assert.Equal(t, "#123456", available[0].Colors.Accent)
}

func TestLoadDoesNotFallBackToEmbeddedThemes(t *testing.T) {
	t.Parallel()

	available, err := Load(fstest.MapFS{}, "")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestLoadRejectsInvalidProvidedTheme(t *testing.T) {
	t.Parallel()

	_, err := Load(fstest.MapFS{
		"Broken.toml": &fstest.MapFile{Data: []byte(`color_scheme = "dark"` + "\n")},
	}, "")

	require.Error(t, err)
	assert.ErrorContains(t, err, "load builtin themes")
	assert.ErrorContains(t, err, "theme Broken.toml")
	assert.ErrorContains(t, err, "colors.")
	assert.ErrorContains(t, err, "is required")
}

func TestLoadSortsProvidedThemes(t *testing.T) {
	t.Parallel()

	available, err := Load(fstest.MapFS{
		"Zulu.toml":  &fstest.MapFile{Data: []byte(testTheme)},
		"alpha.toml": &fstest.MapFile{Data: []byte(testTheme)},
	}, "")

	require.NoError(t, err)
	require.Len(t, available, 2)
	assert.Equal(t, "alpha", available[0].Title)
	assert.Equal(t, "Zulu", available[1].Title)
}

func TestFindIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	available, err := Load(Files, "")
	require.NoError(t, err)

	theme, ok := Find(available, "catppuccin mocha")

	require.True(t, ok)
	assert.Equal(t, "Catppuccin Mocha", theme.Title)
}

func TestLoadOverlaysThemeByFilename(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "Dark.toml"), []byte(testTheme), 0o600))

	available, err := Load(Files, directory)
	require.NoError(t, err)

	theme, ok := Find(available, "dark")

	require.True(t, ok)
	assert.Equal(t, "#123456", theme.Colors.Accent)
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	custom := `color_scheme = "dark"
unknown = "value"
`

	require.NoError(t, os.WriteFile(filepath.Join(directory, "Broken.toml"), []byte(custom), 0o600))

	_, err := Load(Files, directory)

	require.Error(t, err)
}
