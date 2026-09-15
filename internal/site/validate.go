package site

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/icons"
)

// validateConfigFile validates values that came from the TOML configuration file.
func validateConfigFile(config Config) error {
	if err := validateConfigFileValues(config); err != nil {
		return err
	}
	if err := validateAssetPaths(config); err != nil {
		return err
	}
	if err := validateBrandingFormats(config); err != nil {
		return err
	}
	return validateExternalLinks(config.ExternalLinks)
}

// validateResolvedConfig validates relationships after CLI overrides have been applied.
func validateResolvedConfig(config Config) error {
	if err := validateBuildDirectories(config.SourceDir, config.OutputDir); err != nil {
		return err
	}
	return validateAssetPaths(config)
}

// validateConfigFileValues validates TOML values that TinyFlags validates for CLI input.
func validateConfigFileValues(config Config) error {
	if config.SiteName == "" {
		return errors.New("site_name must not be empty")
	}
	if config.SourceDir == "" {
		return errors.New("source_dir must not be empty")
	}
	if config.OutputDir == "" {
		return errors.New("output_dir must not be empty")
	}
	if config.Theme == "" {
		return errors.New("theme must not be empty")
	}
	if config.Language == "" {
		return errors.New("language must not be empty")
	}
	if !domain.ValidNavigationStyle(config.NavigationStyle) {
		return errors.New("navigation_style must be sidebar, topbar, or tree")
	}
	if !domain.ValidNavigationDensity(config.NavigationDensity) {
		return errors.New("navigation_density must be comfortable or compact")
	}
	if err := validateSidebarWidth(config.SidebarWidth); err != nil {
		return err
	}
	if !domain.ValidRobotsPolicy(config.RobotsPolicy) {
		return errors.New("robots must be allow, disallow, or none")
	}

	return nil
}

// validateSidebarWidth checks the supported static navigation width range.
func validateSidebarWidth(width int) error {
	if !domain.ValidSidebarWidth(width) {
		return fmt.Errorf("sidebar_width must be between %d and %d pixels", domain.MinSidebarWidth, domain.MaxSidebarWidth)
	}

	return nil
}

// validateExternalLinks normalizes and validates configurable top-bar links.
func validateExternalLinks(links []domain.ExternalLink) error {
	for index := range links {
		link := &links[index]
		link.Label = strings.TrimSpace(link.Label)
		link.URL = strings.TrimSpace(link.URL)
		link.Icon = strings.TrimSpace(link.Icon)
		link.Description = strings.TrimSpace(link.Description)
		link.HoverEffect = strings.TrimSpace(link.HoverEffect)
		link.HoverText = strings.TrimSpace(link.HoverText)

		if link.Label == "" {
			return fmt.Errorf("external_links[%d].label is required", index)
		}
		if !validExternalLinkURL(link.URL) {
			return fmt.Errorf("external_links[%d].url must be an absolute HTTP or HTTPS URL", index)
		}
		if link.Icon != "" && !icons.ValidName(link.Icon) {
			return fmt.Errorf("external_links[%d].icon must be a valid icon name", index)
		}
		if !domain.ValidExternalLinkHoverEffect(link.HoverEffect) {
			return fmt.Errorf("external_links[%d].hover_effect must be highlight, lift, or none", index)
		}
		link.HoverEffect = domain.EffectiveExternalLinkHoverEffect(link.HoverEffect)
	}

	return nil
}

// validateExternalLinkIcons verifies icon availability after project plugins are loaded.
func validateExternalLinkIcons(links []domain.ExternalLink, catalog *icons.Catalog) error {
	for index, link := range links {
		if icon := strings.TrimSpace(link.Icon); icon != "" && !catalog.IsIcon(icon) {
			return fmt.Errorf("external_links[%d].icon must be an available icon", index)
		}
	}
	return nil
}

// validExternalLinkURL reports whether value is an absolute HTTP or HTTPS URL.
func validExternalLinkURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}

	return strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")
}

// validateBuildDirectories rejects source and output directory overlap.
func validateBuildDirectories(sourceDir, outputDir string) error {
	source, err := filepath.Abs(sourceDir)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	if directoriesOverlap(source, output) {
		return errors.New("source_dir and output_dir must be separate directories")
	}
	return nil
}

// validateAssetPaths checks configured branding files and the optional asset directory.
func validateAssetPaths(config Config) error {
	if err := validateConfiguredPath("logo", config.Logo, config.OutputDir, false); err != nil {
		return err
	}
	if err := validateConfiguredPath("favicon", config.Favicon, config.OutputDir, false); err != nil {
		return err
	}
	if err := validateConfiguredPath("favicon_ico", config.FaviconICO, config.OutputDir, false); err != nil {
		return err
	}
	return validateConfiguredPath("assets_dir", config.AssetsDir, config.OutputDir, true)
}

// validateConfiguredPath checks one configured file or directory and prevents output overlap.
func validateConfiguredPath(name, filename, outputDir string, directory bool) error {
	if filename == "" {
		return nil
	}

	absolute, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	output, err := filepath.Abs(outputDir)
	if err != nil {
		return err
	}
	if pathOverlapsOutput(absolute, output, directory) {
		return fmt.Errorf("%s must be separate from output_dir", name)
	}

	info, err := os.Stat(filename)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if directory && !info.IsDir() {
		return fmt.Errorf("%s must be a directory", name)
	}
	if !directory && !info.Mode().IsRegular() {
		return fmt.Errorf("%s must be a regular file", name)
	}
	return nil
}

// pathOverlapsOutput reports whether a configured path would read from generated output.
func pathOverlapsOutput(pathname, output string, directory bool) bool {
	if pathname == output || directoryContains(output, pathname) {
		return true
	}
	return directory && directoryContains(pathname, output)
}

// validateBrandingFormats checks configured branding file extensions.
func validateBrandingFormats(config Config) error {
	if err := validateImageFormat("logo", config.Logo); err != nil {
		return err
	}
	if err := validateImageFormat("favicon", config.Favicon); err != nil {
		return err
	}
	if config.FaviconICO != "" && !strings.EqualFold(filepath.Ext(config.FaviconICO), ".ico") {
		return errors.New("favicon_ico must be an ICO file")
	}
	return nil
}

// validateImageFormat accepts browser-compatible image extensions for configurable branding.
func validateImageFormat(name, filename string) error {
	if filename == "" {
		return nil
	}

	switch strings.ToLower(filepath.Ext(filename)) {
	case ".svg", ".png", ".jpg", ".jpeg", ".webp", ".gif", ".ico":
		return nil
	default:
		return fmt.Errorf("%s must be an SVG, PNG, JPEG, WebP, GIF, or ICO image", name)
	}
}

// directoriesOverlap reports whether either directory is equal to or contains the other.
func directoriesOverlap(left, right string) bool {
	if left == right {
		return true
	}
	if directoryContains(left, right) {
		return true
	}
	return directoryContains(right, left)
}

// directoryContains reports whether child is nested below parent.
func directoryContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	if err != nil || relative == "." {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
