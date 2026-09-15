package site

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/containeroo/tinyflags"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/logging"
	"github.com/kumbuka-me/kumbuka/internal/pluginproject"
	"github.com/kumbuka-me/kumbuka/themes"
	"github.com/pelletier/go-toml/v2"
)

const defaultConfigPath = "kumbuka-site.toml"

// Config contains filesystem-backed static site build settings.
type Config struct {
	Logo              string                `toml:"logo"`
	Favicon           string                `toml:"favicon"`
	FaviconICO        string                `toml:"favicon_ico"`
	AssetsDir         string                `toml:"assets_dir"`
	SiteName          string                `toml:"site_name"`
	SiteURL           string                `toml:"site_url"`
	SourceDir         string                `toml:"source_dir"`
	OutputDir         string                `toml:"output_dir"`
	Theme             string                `toml:"theme"`
	Language          string                `toml:"language"`
	NavigationStyle   string                `toml:"navigation_style"`
	NavigationDensity string                `toml:"navigation_density"`
	SidebarWidth      int                   `toml:"sidebar_width"`
	RobotsPolicy      string                `toml:"robots"`
	ExternalLinks     []domain.ExternalLink `toml:"external_links"`
	PluginsFile       string                `toml:"-"`
	logFormat         logging.LogFormat
}

// defaultConfig returns generic zero-infrastructure static site defaults.
func defaultConfig() Config {
	preferences := domain.DefaultUserPreferences()

	return Config{
		SiteName:          "Documentation",
		SourceDir:         "docs",
		OutputDir:         "site",
		Theme:             themes.DefaultTheme,
		Language:          "en",
		NavigationStyle:   preferences.NavigationStyle,
		NavigationDensity: preferences.NavigationDensity,
		SidebarWidth:      preferences.SidebarWidth,
		RobotsPolicy:      domain.RobotsPolicyAllow,
		PluginsFile:       pluginproject.DefaultFile,
		logFormat:         logging.LogFormatJSON,
	}
}

// loadConfig reads an optional TOML site configuration and resolves config-relative asset paths.
func loadConfig(filename string, required bool) (Config, error) {
	config := defaultConfig()
	file, err := os.Open(filename)
	if errors.Is(err, os.ErrNotExist) && !required {
		return config, nil
	}
	if err != nil {
		return Config{}, err
	}
	defer file.Close() // nolint:errcheck

	if err := toml.NewDecoder(file).DisallowUnknownFields().Decode(&config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", filename, err)
	}

	config.resolveAssetPaths(filepath.Dir(filename))
	if err := validateConfigFile(config); err != nil {
		return Config{}, err
	}

	return config, nil
}

// resolveAssetPaths resolves user-supplied branding and asset paths relative to the config file.
func (c *Config) resolveAssetPaths(configDir string) {
	resolveRelativePath(configDir, &c.Logo)
	resolveRelativePath(configDir, &c.Favicon)
	resolveRelativePath(configDir, &c.FaviconICO)
	resolveRelativePath(configDir, &c.AssetsDir)
}

// resolveRelativePath resolves one non-empty relative path against the supplied directory.
func resolveRelativePath(baseDir string, filename *string) {
	if *filename == "" || filepath.IsAbs(*filename) {
		return
	}
	*filename = filepath.Clean(filepath.Join(baseDir, *filename))
}

// BindFlags registers static site flags and returns a resolver for the effective configuration.
func BindFlags(flags *tinyflags.FlagSet) func() (Config, error) {
	defaults := defaultConfig()

	configPath := flags.String("config", defaultConfigPath, "TOML site configuration file").
		Placeholder("FILE")
	siteName := flags.String("site-name", defaults.SiteName, "Site title").
		NotEmpty().
		Placeholder("NAME")
	siteURL := flags.String("site-url", defaults.SiteURL, "Published site URL").Placeholder("URL")
	source := flags.String("source", defaults.SourceDir, "Markdown source directory").
		NotEmpty().
		Placeholder("DIR")
	output := flags.String("output", defaults.OutputDir, "Generated site directory").
		NotEmpty().
		Placeholder("DIR")
	theme := flags.String("theme", defaults.Theme, "Theme").
		NotEmpty().
		Placeholder("THEME")
	language := flags.String("language", defaults.Language, "HTML content language").
		NotEmpty().
		Placeholder("LANG")
	navigationStyle := flags.String("navigation-style", defaults.NavigationStyle, "Desktop navigation style").
		Choices(domain.NavigationStyleSidebar, domain.NavigationStyleTopbar, domain.NavigationStyleTree).
		Placeholder("STYLE")
	navigationDensity := flags.String("navigation-density", defaults.NavigationDensity, "Navigation density").
		Choices(domain.NavigationDensityComfortable, domain.NavigationDensityCompact).
		Placeholder("DENSITY")
	sidebarWidth := flags.Int("sidebar-width", defaults.SidebarWidth, "Desktop sidebar width in pixels").
		Validate(validateSidebarWidth).
		Placeholder("PIXELS")
	pluginsFile := flags.String("plugins", defaults.PluginsFile, "Static plugin dependency file").
		NotEmpty().
		Placeholder("FILE")
	robots := flags.String("robots", defaults.RobotsPolicy, "robots.txt policy").
		Choices(domain.RobotsPolicyAllow, domain.RobotsPolicyDisallow, domain.RobotsPolicyNone).
		Placeholder("POLICY")
	logFormat := flags.String("log-format", string(defaults.logFormat), "Log output format").
		Choices(string(logging.LogFormatText), string(logging.LogFormatJSON)).
		Short("l").
		Placeholder("FORMAT")

	return func() (Config, error) {
		cfg, err := loadConfig(*configPath.Value(), configPath.Changed())
		if err != nil {
			return Config{}, err
		}

		if siteName.Changed() {
			cfg.SiteName = *siteName.Value()
		}
		if siteURL.Changed() {
			cfg.SiteURL = *siteURL.Value()
		}
		if source.Changed() {
			cfg.SourceDir = *source.Value()
		}
		if output.Changed() {
			cfg.OutputDir = *output.Value()
		}
		if theme.Changed() {
			cfg.Theme = *theme.Value()
		}
		if language.Changed() {
			cfg.Language = *language.Value()
		}
		if navigationStyle.Changed() {
			cfg.NavigationStyle = *navigationStyle.Value()
		}
		if navigationDensity.Changed() {
			cfg.NavigationDensity = *navigationDensity.Value()
		}
		if sidebarWidth.Changed() {
			cfg.SidebarWidth = *sidebarWidth.Value()
		}
		if pluginsFile.Changed() {
			cfg.PluginsFile = *pluginsFile.Value()
		}
		if robots.Changed() {
			cfg.RobotsPolicy = *robots.Value()
		}
		cfg.logFormat = logging.LogFormat(*logFormat.Value())

		if err := validateResolvedConfig(cfg); err != nil {
			return Config{}, err
		}

		return cfg, nil
	}
}
