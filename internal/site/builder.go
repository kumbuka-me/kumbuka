package site

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"slices"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/icons"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugincap"
	"github.com/kumbuka-me/kumbuka/themes"
)

// builder converts Markdown files into one read-only static site.
type builder struct {
	appFS       fs.FS
	renderer    *md.Renderer
	iconCatalog *icons.Catalog
}

// buildResult summarizes one completed static build.
type buildResult struct {
	pages     int
	outputDir string
}

// sourcePage contains one discovered Markdown page and its generated data.
type sourcePage struct {
	SourcePath      string
	Route           string
	Title           string
	Markdown        string
	HasTitleHeading bool
	HTML            template.HTML
	Contents        []md.Heading
	SearchText      string
}

// searchEntry is one browser-side static search document.
type searchEntry struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Text  string `json:"text"`
}

// viewData contains the shared data rendered by static site templates.
type viewData struct {
	LogoURL           string
	FaviconURL        string
	FaviconICOURL     string
	SiteName          string
	SiteURL           string
	BasePath          string
	Language          string
	Title             string
	ActiveTheme       string
	NavigationStyle   string
	NavigationDensity string
	SidebarWidth      int
	ThemeData         template.JS
	PluginModules     template.JS
	CurrentRoute      string
	Navigation        []navigation.Node
	HTML              template.HTML
	PageContents      []md.Heading
	ExternalLinks     []domain.ExternalLink
}

// buildPlan contains validated and precomputed state shared by one build.
type buildPlan struct {
	config          Config
	basePath        string
	pages           []sourcePage
	routesBySource  map[string]string
	wikiTargets     map[string]string
	navigationPages []navigation.Page
	navigationTree  []navigation.Node
	themeData       template.JS
	templates       siteTemplates
}

// renderedPage contains one processed page body plus search and contents data.
type renderedPage struct {
	html       string
	searchText string
	contents   []md.Heading
}

// newBuilder constructs the filesystem-backed static site builder.
func newBuilder(appFS fs.FS) *builder {
	return &builder{
		appFS:       appFS,
		iconCatalog: icons.Builtin(),
	}
}

// build renders all configured Markdown files into the configured output directory.
func (b *builder) build(ctx context.Context, config Config) (buildResult, error) {
	// Copy the builder so a scoped project renderer is never retained after close.
	local := *b
	b = &local
	plan, err := b.planBuild(config)
	if err != nil {
		return buildResult{}, err
	}
	if b.renderer == nil {
		renderer, err := projectRenderer(ctx, config.PluginsFile, plan.pages)
		if err != nil {
			return buildResult{}, err
		}
		defer func() { _ = renderer.Close(context.Background()) }()
		b.renderer = renderer
	}

	b.iconCatalog = b.renderer.IconCatalog()
	if err := validateExternalLinkIcons(plan.config.ExternalLinks, b.iconCatalog); err != nil {
		return buildResult{}, err
	}
	plan.templates, err = b.parseTemplates(plan.basePath)
	if err != nil {
		return buildResult{}, err
	}

	branding, err := b.prepareOutput(plan.config, plan.basePath)
	if err != nil {
		return buildResult{}, err
	}
	pluginModules, err := b.pluginModulesJSON(plan.basePath + "plugins")
	if err != nil {
		return buildResult{}, err
	}

	common := commonViewData(plan, branding)
	common.PluginModules = pluginModules
	searchIndex, err := b.renderPages(ctx, plan, common)
	if err != nil {
		return buildResult{}, err
	}
	if err := b.writeSupportFiles(plan, common, searchIndex); err != nil {
		return buildResult{}, err
	}

	return buildResult{pages: len(plan.pages), outputDir: plan.config.OutputDir}, nil
}

// planBuild validates configuration and prepares immutable state used by rendering.
func (b *builder) planBuild(config Config) (buildPlan, error) {
	if err := validateResolvedConfig(config); err != nil {
		return buildPlan{}, err
	}

	themeData, err := loadThemeData(config.Theme)
	if err != nil {
		return buildPlan{}, err
	}

	pages, err := discoverPages(config.SourceDir)
	if err != nil {
		return buildPlan{}, err
	}
	if len(pages) == 0 {
		return buildPlan{}, fmt.Errorf("no Markdown files found in %s", config.SourceDir)
	}
	if !hasHomePage(pages) {
		return buildPlan{}, fmt.Errorf("%s must contain index.md for the site home page", config.SourceDir)
	}

	basePath, err := staticBasePath(config.SiteURL)
	if err != nil {
		return buildPlan{}, err
	}

	routesBySource, wikiTargets := indexPages(pages)
	navigationPages := buildNavigationPages(pages)
	templates, err := b.parseTemplates(basePath)
	if err != nil {
		return buildPlan{}, err
	}

	return buildPlan{
		config:          config,
		basePath:        basePath,
		pages:           pages,
		routesBySource:  routesBySource,
		wikiTargets:     wikiTargets,
		navigationPages: navigationPages,
		navigationTree:  navigation.Build(navigationPages, navigation.Options{}),
		themeData:       themeData,
		templates:       templates,
	}, nil
}

// loadThemeData validates the selected theme and serializes the available theme catalog.
func loadThemeData(theme string) (template.JS, error) {
	availableThemes, err := themes.Load("")
	if err != nil {
		return "", err
	}
	if _, found := themes.Find(availableThemes, theme); !found {
		return "", fmt.Errorf("unknown theme %q", theme)
	}

	data, err := json.Marshal(availableThemes)
	if err != nil {
		return "", err
	}

	return template.JS(data), nil
}

// buildNavigationPages converts discovered source pages into navigation inputs.
func buildNavigationPages(pages []sourcePage) []navigation.Page {
	navigationPages := make([]navigation.Page, 0, len(pages))
	for _, page := range pages {
		if page.Route == "" {
			continue
		}
		navigationPages = append(navigationPages, navigation.Page{Slug: page.Route, Title: page.Title})
	}

	return navigationPages
}

// commonViewData assembles template data shared by every generated page.
func commonViewData(plan buildPlan, branding brandingData) viewData {
	return viewData{
		LogoURL:           branding.LogoURL,
		FaviconURL:        branding.FaviconURL,
		FaviconICOURL:     branding.FaviconICOURL,
		SiteName:          plan.config.SiteName,
		SiteURL:           plan.config.SiteURL,
		BasePath:          plan.basePath,
		Language:          plan.config.Language,
		ActiveTheme:       plan.config.Theme,
		NavigationStyle:   plan.config.NavigationStyle,
		NavigationDensity: plan.config.NavigationDensity,
		SidebarWidth:      plan.config.SidebarWidth,
		ThemeData:         plan.themeData,
		ExternalLinks:     slices.Clone(plan.config.ExternalLinks),
	}
}

// renderPages renders each discovered source page and builds the static search index.
func (b *builder) renderPages(ctx context.Context, plan buildPlan, common viewData) ([]searchEntry, error) {
	searchIndex := make([]searchEntry, 0, len(plan.pages))
	for index := range plan.pages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		page := &plan.pages[index]
		if err := validateWikiLinks(*page, plan.wikiTargets); err != nil {
			return nil, err
		}

		rendered, err := b.renderPage(ctx, *page, plan)
		if err != nil {
			return nil, err
		}
		page.HTML = template.HTML(rendered.html)
		page.SearchText = rendered.searchText
		page.Contents = rendered.contents

		data := pageViewData(common, *page, plan.navigationPages)
		if err := writeTemplate(plan.templates.page, outputFilename(plan.config.OutputDir, page.Route), data); err != nil {
			return nil, err
		}

		searchIndex = append(searchIndex, searchEntry{
			Title: page.Title,
			URL:   pageURL(plan.basePath, page.Route),
			Text:  page.SearchText,
		})
	}

	return searchIndex, nil
}

// renderPage renders one source page with shared Markdown functions and static URL rewriting.
func (b *builder) renderPage(ctx context.Context, page sourcePage, plan buildPlan) (renderedPage, error) {
	options := md.DefaultOptions()
	options.WikiLinkPrefix = plan.basePath
	resolveWiki := func(target string) string {
		normalized := md.Slug(target)
		if route, found := plan.wikiTargets[normalized]; found {
			return routeSuffix(route)
		}
		return routeSuffix(normalized)
	}

	children := plan.navigationTree
	if page.Route != "" {
		children = navigation.Children(plan.navigationTree, page.Route)
	}
	pageNavigation := plugincap.Navigation(children, func(slug string) string {
		return pageURL(plan.basePath, slug)
	})
	rendered, err := b.renderer.RenderPageResolvedWithFunctions(
		page.Markdown,
		resolveWiki,
		options,
		md.Functions{Context: ctx, Capabilities: plugincap.Capabilities(nil, pageNavigation, b.iconCatalog)},
	)
	if err != nil {
		return renderedPage{}, fmt.Errorf("render %s: %w", page.SourcePath, err)
	}

	html, searchText, err := processRenderedHTML(
		rendered.HTML,
		page.SourcePath,
		page.HasTitleHeading,
		plan.routesBySource,
		plan.basePath,
	)
	if err != nil {
		return renderedPage{}, fmt.Errorf("rewrite %s: %w", page.SourcePath, err)
	}

	contents := rendered.Contents
	if page.HasTitleHeading && len(contents) > 0 && contents[0].Level == 1 {
		contents = contents[1:]
	}

	return renderedPage{html: html, searchText: searchText, contents: contents}, nil
}

// pageViewData adds page-specific values to the shared static template data.
func pageViewData(common viewData, page sourcePage, navigationPages []navigation.Page) viewData {
	data := common
	data.Title = page.Title
	data.CurrentRoute = page.Route
	data.Navigation = navigation.Build(navigationPages, navigation.Options{
		ActiveSlug: page.Route,
		Expanded:   expandedPrefixes(page.Route),
	})
	data.HTML = page.HTML
	data.PageContents = page.Contents
	return data
}

// writeSupportFiles writes the static search page, error page, search index, and sitemap.
func (b *builder) writeSupportFiles(plan buildPlan, common viewData, searchIndex []searchEntry) error {
	slices.SortFunc(searchIndex, compareSearchEntries)
	if err := writeJSON(outputFile(plan.config.OutputDir, "search-index.json"), searchIndex); err != nil {
		return err
	}
	if err := writeSearchPage(plan, common); err != nil {
		return err
	}
	if err := writeNotFoundPage(plan, common); err != nil {
		return err
	}
	if err := writeSitemap(plan.config, plan.pages); err != nil {
		return err
	}
	return writeRobots(plan.config)
}

// writeSearchPage renders the browser-side static search page.
func writeSearchPage(plan buildPlan, common viewData) error {
	data := common
	data.Title = "Search"
	data.Navigation = navigation.Build(plan.navigationPages, navigation.Options{})
	return writeTemplate(plan.templates.search, outputFile(plan.config.OutputDir, "search", "index.html"), data)
}

// writeNotFoundPage renders the static 404 page.
func writeNotFoundPage(plan buildPlan, common viewData) error {
	data := common
	data.Title = "Page not found"
	data.Navigation = navigation.Build(plan.navigationPages, navigation.Options{})
	return writeTemplate(plan.templates.notFound, outputFile(plan.config.OutputDir, "404.html"), data)
}

// BuildWithRenderer lets an application build a site using its active plugin
// registry, including runtime-installed plugins. The caller owns the renderer.
func BuildWithRenderer(ctx context.Context, appFS fs.FS, config Config, renderer *md.Renderer) error {
	if renderer == nil {
		return fmt.Errorf("site renderer is required")
	}
	b := newBuilder(appFS)
	b.renderer = renderer
	_, err := b.build(ctx, config)
	return err
}
