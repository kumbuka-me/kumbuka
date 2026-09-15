package site

import (
	"bytes"
	"cmp"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
)

//go:embed templates/*.gohtml
var templateFiles embed.FS

type siteTemplates struct {
	page     *template.Template
	search   *template.Template
	notFound *template.Template
}

// parseTemplates parses every static page template against the shared layout and helpers.
func (b *builder) parseTemplates(basePath string) (siteTemplates, error) {
	page, err := b.parseTemplate("page.gohtml", basePath)
	if err != nil {
		return siteTemplates{}, err
	}

	search, err := b.parseTemplate("search.gohtml", basePath)
	if err != nil {
		return siteTemplates{}, err
	}

	notFound, err := b.parseTemplate("not_found.gohtml", basePath)
	if err != nil {
		return siteTemplates{}, err
	}

	return siteTemplates{page: page, search: search, notFound: notFound}, nil
}

// parseTemplate parses one static page template with URL and icon helpers.
func (b *builder) parseTemplate(pageTemplate, basePath string) (*template.Template, error) {
	funcs := template.FuncMap{
		"icon":          b.iconCatalog.SVG,
		"externalhover": domain.ExternalLinkHoverTitle,
		"externalhovereffect": func(link domain.ExternalLink) string {
			return domain.EffectiveExternalLinkHoverEffect(link.HoverEffect)
		},
		"pageurl": func(route string) string {
			return pageURL(basePath, route)
		},
		"asseturl": func(name string) string {
			return basePath + "assets/" + strings.TrimPrefix(name, "/")
		},
		"searchurl": func() string {
			return basePath + "search/"
		},
	}

	return template.New("static").Funcs(funcs).ParseFS(
		templateFiles,
		"templates/layout.gohtml",
		"templates/navigation.gohtml",
		"templates/"+pageTemplate,
	)
}

// staticBasePath derives the generated URL prefix from the configured site URL.
func staticBasePath(siteURL string) (string, error) {
	if strings.TrimSpace(siteURL) == "" {
		return "/", nil
	}

	parsed, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil {
		return "", fmt.Errorf("parse site_url: %w", err)
	}

	base := cmp.Or(parsed.Path, "/")
	if !strings.HasPrefix(base, "/") {
		base = "/" + base
	}
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}

	cleaned := path.Clean(base)
	if cleaned == "/" {
		return "/", nil
	}

	return strings.TrimSuffix(cleaned, "/") + "/", nil
}

// pageURL returns the clean public URL for one generated page route.
func pageURL(basePath, route string) string {
	basePath = ensureBasePath(basePath)
	route = strings.Trim(route, "/")
	if route == "" {
		return basePath
	}

	return basePath + route + "/"
}

// ensureBasePath normalizes a URL prefix to one leading and trailing slash.
func ensureBasePath(basePath string) string {
	if basePath == "" || basePath == "." {
		return "/"
	}

	basePath = "/" + strings.Trim(basePath, "/")
	if basePath == "/" {
		return basePath
	}

	return basePath + "/"
}

// routeSuffix converts one normalized route into its directory-style suffix.
func routeSuffix(route string) string {
	route = strings.Trim(route, "/")
	if route == "" {
		return ""
	}

	return route + "/"
}

// outputPath maps one page route to its generated HTML path.
func outputPath(route string) string {
	if strings.Trim(route, "/") == "" {
		return "index.html"
	}

	return filepath.Join(filepath.FromSlash(strings.Trim(route, "/")), "index.html")
}

// outputFilename returns the generated HTML filename for one route.
func outputFilename(outputDir, route string) string {
	return outputFile(outputDir, outputPath(route))
}

// outputFile joins an output directory with generated path components.
func outputFile(outputDir string, parts ...string) string {
	all := make([]string, 0, len(parts)+1)
	all = append(all, outputDir)
	all = append(all, parts...)
	return filepath.Join(all...)
}

// writeTemplate renders one complete static HTML page through an in-memory buffer before writing it.
func writeTemplate(tmpl *template.Template, filename string, data viewData) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}

	var output bytes.Buffer
	if err := tmpl.ExecuteTemplate(&output, "layout", data); err != nil {
		return err
	}

	return os.WriteFile(filename, output.Bytes(), 0o644)
}

// writeJSON writes indented JSON terminated by a newline.
func writeJSON(filename string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	return os.WriteFile(filename, data, 0o644)
}

// writeSitemap writes sitemap.xml when site_url is an absolute HTTP or HTTPS URL.
func writeSitemap(config Config, pages []sourcePage) error {
	if strings.TrimSpace(config.SiteURL) == "" {
		return nil
	}

	parsed, err := url.Parse(config.SiteURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil
	}

	basePath, err := staticBasePath(config.SiteURL)
	if err != nil {
		return err
	}

	origin := parsed.Scheme + "://" + parsed.Host
	var output strings.Builder
	output.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	output.WriteString("<urlset xmlns=\"http://www.sitemaps.org/schemas/sitemap/0.9\">\n")
	for _, page := range pages {
		output.WriteString("  <url><loc>")
		output.WriteString(template.HTMLEscapeString(origin + pageURL(basePath, page.Route)))
		output.WriteString("</loc></url>\n")
	}
	output.WriteString("</urlset>\n")

	return os.WriteFile(outputFile(config.OutputDir, "sitemap.xml"), []byte(output.String()), 0o644)
}

// writeRobots writes robots.txt according to the configured static-site policy.
func writeRobots(config Config) error {
	if config.RobotsPolicy == domain.RobotsPolicyNone {
		return nil
	}

	var output strings.Builder
	output.WriteString("User-agent: *\n")

	switch config.RobotsPolicy {
	case domain.RobotsPolicyAllow:
		output.WriteString("Allow: /\n")
		if sitemap := staticSitemapURL(config.SiteURL); sitemap != "" {
			output.WriteString("Sitemap: ")
			output.WriteString(sitemap)
			output.WriteByte('\n')
		}
	case domain.RobotsPolicyDisallow:
		output.WriteString("Disallow: /\n")
	default:
		return fmt.Errorf("unsupported robots policy %q", config.RobotsPolicy)
	}

	return os.WriteFile(outputFile(config.OutputDir, "robots.txt"), []byte(output.String()), 0o644)
}

// staticSitemapURL returns the public sitemap URL when site_url is absolute.
func staticSitemapURL(siteURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(siteURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}

	basePath, err := staticBasePath(siteURL)
	if err != nil {
		return ""
	}

	parsed.Path = path.Join(basePath, "sitemap.xml")
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String()
}

// compareSearchEntries orders search results by case-insensitive page title.
func compareSearchEntries(left, right searchEntry) int {
	return cmp.Compare(strings.ToLower(left.Title), strings.ToLower(right.Title))
}
