package themes

import (
	"cmp"
	"embed"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

//go:embed *.toml
var embeddedFiles embed.FS

// DefaultTheme is the embedded fallback used before a user selects a preference.
const DefaultTheme = "Light"

// Theme defines one file-backed theme available to the Kumbuka interface.
type Theme struct {
	// Title is derived from the theme filename without its extension.
	Title string `json:"title"        toml:"-"`
	// ColorScheme controls browser-native light or dark rendering.
	ColorScheme string `json:"color_scheme" toml:"color_scheme"`
	// Colors contains the semantic colors consumed by Kumbuka components.
	Colors Colors `json:"colors"       toml:"colors"`
}

// Colors defines the semantic color roles understood by the Kumbuka interface.
type Colors struct {
	// Background is the application canvas behind all content.
	Background string `json:"background"       toml:"background"`
	// Surface is the default container and control background.
	Surface string `json:"surface"          toml:"surface"`
	// SurfaceElevated is used for visually raised or emphasized surfaces.
	SurfaceElevated string `json:"surface_elevated" toml:"surface_elevated"`
	// SurfaceHover is used when an interactive surface is hovered.
	SurfaceHover string `json:"surface_hover"    toml:"surface_hover"`

	// Text is the primary foreground color.
	Text string `json:"text"           toml:"text"`
	// TextSecondary is used for supporting foreground content.
	TextSecondary string `json:"text_secondary" toml:"text_secondary"`
	// TextTertiary is used for low-emphasis metadata.
	TextTertiary string `json:"text_tertiary"  toml:"text_tertiary"`
	// Muted is used for subdued labels and placeholder content.
	Muted string `json:"muted"          toml:"muted"`

	// Border is the default separator and control border color.
	Border string `json:"border"        toml:"border"`
	// BorderStrong is used for emphasized borders and focus states.
	BorderStrong string `json:"border_strong" toml:"border_strong"`
	// BorderSubtle is used for low-emphasis separators.
	BorderSubtle string `json:"border_subtle" toml:"border_subtle"`

	// Accent is the primary interactive and link color.
	Accent string `json:"accent"           toml:"accent"`
	// AccentSecondary is the secondary interactive accent color.
	AccentSecondary string `json:"accent_secondary" toml:"accent_secondary"`
	// AccentSoft is the low-emphasis accent used for decorative highlights.
	AccentSoft string `json:"accent_soft"      toml:"accent_soft"`

	// Success indicates successful or helpful states.
	Success string `json:"success" toml:"success"`
	// Warning indicates cautionary states.
	Warning string `json:"warning" toml:"warning"`
	// Error indicates validation and request errors.
	Error string `json:"error"   toml:"error"`
	// Danger indicates destructive actions or critical states.
	Danger string `json:"danger"  toml:"danger"`

	// SelectionText is the foreground color for selected text.
	SelectionText string `json:"selection_text"       toml:"selection_text"`
	// SelectionBackground is the background color for selected text.
	SelectionBackground string `json:"selection_background" toml:"selection_background"`
}

// Load reads embedded themes and overlays themes from an optional directory.
func Load(directory string) ([]Theme, error) {
	available, err := loadFS(embeddedFiles, ".")
	if err != nil {
		return nil, fmt.Errorf("load embedded themes: %w", err)
	}
	if directory == "" {
		return available, nil
	}

	external, err := loadFS(os.DirFS(directory), ".")
	if err != nil {
		return nil, fmt.Errorf("load themes from %s: %w", directory, err)
	}

	return merge(available, external), nil
}

// Find returns the named theme using a case-insensitive filename match.
func Find(available []Theme, name string) (theme Theme, found bool) {
	for _, theme := range available {
		if strings.EqualFold(theme.Title, name) {
			return theme, true
		}
	}

	return Theme{}, false
}

// loadFS loads all TOML theme files from the provided filesystem root.
func loadFS(source fs.FS, root string) ([]Theme, error) {
	entries, err := fs.ReadDir(source, root)
	if err != nil {
		return nil, err
	}

	var available []Theme

	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(path.Ext(entry.Name()), ".toml") {
			continue
		}

		file, err := source.Open(path.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}

		var theme Theme
		decodeErr := toml.NewDecoder(file).DisallowUnknownFields().Decode(&theme)
		closeErr := file.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("parse theme %s: %w", entry.Name(), decodeErr)
		}
		if closeErr != nil {
			return nil, closeErr
		}

		theme.Title = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		if err := validate(theme); err != nil {
			return nil, fmt.Errorf("theme %s: %w", entry.Name(), err)
		}

		available = append(available, theme)
	}

	sortThemes(available)
	return available, nil
}

// merge overlays themes with matching titles and preserves deterministic ordering.
func merge(base, overlays []Theme) []Theme {
	byName := make(map[string]Theme, len(base)+len(overlays))

	for _, theme := range base {
		byName[strings.ToLower(theme.Title)] = theme
	}
	for _, theme := range overlays {
		byName[strings.ToLower(theme.Title)] = theme
	}

	return slices.SortedFunc(maps.Values(byName), compareThemes)
}

// sortThemes sorts themes case-insensitively by their filename-derived title.
func sortThemes(available []Theme) {
	slices.SortFunc(available, compareThemes)
}

// validate ensures a theme provides every required semantic color.
func validate(theme Theme) error {
	if theme.ColorScheme != "light" && theme.ColorScheme != "dark" {
		return fmt.Errorf("color_scheme must be light or dark")
	}

	required := map[string]string{
		"background":           theme.Colors.Background,
		"surface":              theme.Colors.Surface,
		"surface_elevated":     theme.Colors.SurfaceElevated,
		"surface_hover":        theme.Colors.SurfaceHover,
		"text":                 theme.Colors.Text,
		"text_secondary":       theme.Colors.TextSecondary,
		"text_tertiary":        theme.Colors.TextTertiary,
		"muted":                theme.Colors.Muted,
		"border":               theme.Colors.Border,
		"border_strong":        theme.Colors.BorderStrong,
		"border_subtle":        theme.Colors.BorderSubtle,
		"accent":               theme.Colors.Accent,
		"accent_secondary":     theme.Colors.AccentSecondary,
		"accent_soft":          theme.Colors.AccentSoft,
		"success":              theme.Colors.Success,
		"warning":              theme.Colors.Warning,
		"error":                theme.Colors.Error,
		"danger":               theme.Colors.Danger,
		"selection_text":       theme.Colors.SelectionText,
		"selection_background": theme.Colors.SelectionBackground,
	}

	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("colors.%s is required", name)
		}
	}

	return nil
}

// compareThemes compares filename-derived titles case-insensitively.
func compareThemes(left, right Theme) int {
	return cmp.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
}
