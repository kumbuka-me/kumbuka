package themes

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEmbeddedThemeCatalog(t *testing.T) {
	t.Parallel()
	available, err := Load("")
	require.NoError(t, err)
	assert.Len(t, available, 16)

	t.Run("Catppuccin Frappe", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Catppuccin Frappe")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Catppuccin Latte", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Catppuccin Latte")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "light", theme.ColorScheme)
	})

	t.Run("Catppuccin Macchiato", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Catppuccin Macchiato")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Catppuccin Mocha", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Catppuccin Mocha")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Dark", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Dark")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Dracula", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Dracula")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Gruvbox Dark", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Gruvbox Dark")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Gruvbox Light", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Gruvbox Light")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "light", theme.ColorScheme)
	})

	t.Run("Light", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Light")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "light", theme.ColorScheme)
	})

	t.Run("Nord", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Nord")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("One Dark", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "One Dark")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Rose Pine", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Rose Pine")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Rose Pine Dawn", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Rose Pine Dawn")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "light", theme.ColorScheme)
	})

	t.Run("Solarized Dark", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Solarized Dark")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})

	t.Run("Solarized Light", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Solarized Light")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "light", theme.ColorScheme)
	})

	t.Run("Tokyo Night", func(t *testing.T) {
		t.Parallel()

		theme, ok := Find(available, "Tokyo Night")
		require.True(t, ok, "embedded theme is missing")
		assert.Equal(t, "dark", theme.ColorScheme)
	})
}

func TestFindIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	available, err := Load("")

	require.NoError(t, err)

	theme, ok := Find(available, "catppuccin mocha")

	require.True(t, ok)
	assert.Equal(t, "Catppuccin Mocha", theme.Title)
}

func TestLoadOverlaysThemeByFilename(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	custom := `color_scheme = "dark"

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

	require.NoError(t, os.WriteFile(filepath.Join(directory, "Dark.toml"), []byte(custom), 0o600))

	available, err := Load(directory)

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

	_, err := Load(directory)

	require.Error(t, err)
}
