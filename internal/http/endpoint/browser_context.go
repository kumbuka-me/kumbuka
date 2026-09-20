package endpoint

import (
	"encoding/json"
	"github.com/kumbuka-me/kumbuka/internal/application/viewer"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"html/template"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginbrowser"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

// BrowserContext assembles browser presentation at the HTTP orchestration boundary.
type BrowserContext struct {
	query         *viewer.Query
	pluginManager *plugin.Manager
	renderer      *md.Renderer
}

// NewBrowserContext wires shared application data and plugin presentation.
func NewBrowserContext(query *viewer.Query, renderer *md.Renderer) *BrowserContext {
	var manager *plugin.Manager
	if renderer != nil {
		manager = renderer.PluginManager()
	}
	return &BrowserContext{query: query, pluginManager: manager, renderer: renderer}
}

// pluginData groups plugin-owned contributions shared by authenticated templates.
type pluginData struct {
	// features contains enabled plugin and setting flags.
	features map[string]bool
	// widgetPreferences contains user-facing visibility controls.
	widgetPreferences []webview.WidgetPreference
	// modules contains the serialized browser-module catalog.
	modules template.JS
	// stylesVersion fingerprints active plugin presentation styles.
	stylesVersion string
	// editorInserts contains active editor actions.
	editorInserts []plugin.EditorInsertContribution
	// sidebarWidgets contains rendered sidebar widget models.
	sidebarWidgets []webview.Widget
	// settingsLinks contains installed plugins with administrator configuration.
	settingsLinks []webview.PluginSettingsLink
}

// Load builds the common template data used by every authenticated browser page.
func (l *BrowserContext) Load(r *http.Request, views *webview.Views, title string) (webview.Layout, error) {
	user, _ := auth.User(r)

	context, err := l.query.Load(r.Context(), user, !strings.HasPrefix(r.URL.Path, "/admin"))
	if err != nil {
		return webview.Layout{}, err
	}
	preferences := context.Preferences
	applicationSettings := context.Settings
	pageNavigation := presentNavigation(context, r.URL.Path)
	typographySize := effectiveTypographySize(preferences, applicationSettings)
	activeTheme := selectedTheme(views, preferences.Theme)
	preferences.Theme = activeTheme

	stop := measurePageStage(r.Context(), "view_theme_data")
	themeData, err := json.Marshal(views.Themes())
	stop()
	if err != nil {
		return webview.Layout{}, err
	}

	plugins, err := l.loadPluginData(r, user, preferences)
	if err != nil {
		return webview.Layout{}, err
	}

	return webview.Layout{
		Title:               title,
		User:                user,
		Preferences:         preferences,
		TypographySize:      typographySize,
		Navigation:          pageNavigation,
		NewPageParent:       activeNavigationSlug(r.URL.Path),
		SidebarWidgets:      plugins.sidebarWidgets,
		SavedSearches:       context.SavedSearches,
		Notifications:       context.Notifications,
		UnreadNotifications: context.UnreadNotifications,

		Version:                 views.Version(),
		AssetVersion:            views.AssetVersion(),
		Commit:                  views.Commit(),
		Runtime:                 views.Runtime(),
		ThemeData:               template.JS(themeData),
		Themes:                  views.Themes(),
		ActiveTheme:             activeTheme,
		ApplicationSettings:     applicationSettings,
		PluginFeatures:          plugins.features,
		PluginWidgetPreferences: plugins.widgetPreferences,
		PluginModules:           plugins.modules,
		PluginStylesVersion:     plugins.stylesVersion,
		EditorInserts:           plugins.editorInserts,
		PluginSettingsLinks:     plugins.settingsLinks,
		CanEdit:                 user.CanEditContent(),
	}, nil
}

// presentNavigation converts an already-authorized page collection into the browser tree.
func presentNavigation(context viewer.Context, requestPath string) []navigation.Node {
	preferences := context.Preferences
	pages := context.Pages
	navigationIcons := context.NavigationIcons
	expanded := preferences.ExpandedNavigation
	if !preferences.RememberNavigationState {
		expanded = nil
	}

	navigationPages := make([]navigation.Page, 0, len(pages))
	for _, page := range pages {
		navigationPages = append(navigationPages, navigation.Page{
			Slug:  page.Slug,
			Title: page.Title,
			Icon:  page.Icon,
		})
	}

	result := navigation.Build(navigationPages, navigation.Options{
		ActiveSlug:     activeNavigationSlug(requestPath),
		Expanded:       expanded,
		ShowPageCounts: preferences.ShowNavigationPageCounts,
		Icons:          navigationIcons,
	})
	return result
}

// addPluginFeatures records namespaced plugin flags and generic Markdown capabilities.
func addPluginFeatures(features map[string]bool, item plugin.LoadedPlugin) {
	features[item.Manifest.ID] = item.Enabled
	if !item.Enabled {
		return
	}

	for key, enabled := range item.Settings {
		features[item.Manifest.ID+"."+key] = enabled
	}

	for _, module := range item.Manifest.Modules {
		if module.Type != "markdown-syntax" || module.Syntax == "" {
			continue
		}

		feature := "markdown-syntax." + module.Syntax
		features[feature] = true
		for _, setting := range item.Manifest.Modules {
			if setting.Type != "settings" || !slices.Contains(setting.Requires, module.ID) {
				continue
			}
			features[feature+"."+setting.ID] = item.Settings[setting.ID]
		}
	}
}

// loadPluginData resolves active plugin flags, browser assets, editor actions, and sidebar widgets.
func (l *BrowserContext) loadPluginData(
	r *http.Request,
	user domain.User,
	preferences domain.UserPreferences,
) (pluginData, error) {
	stop := measurePageStage(r.Context(), "view_plugin_features")
	features := make(map[string]bool)
	var editorInserts []plugin.EditorInsertContribution
	var loadedPlugins []plugin.LoadedPlugin
	if l.pluginManager != nil {
		editorInserts = l.pluginManager.EditorInserts()
		loadedPlugins = l.pluginManager.Plugins()
		for _, item := range loadedPlugins {
			addPluginFeatures(features, item)
		}
	}
	stop()

	var sidebarWidgets []webview.Widget
	if !strings.HasPrefix(r.URL.Path, "/admin") && l.renderer != nil {
		stop = measurePageStage(r.Context(), "view_sidebar_widgets")
		source := l.query.PersonalLists(user)
		capabilities := plugincap.MergeCapabilities(
			plugincap.Capabilities(nil, nil, l.renderer.IconCatalog()),
			plugincap.PageListCapabilities(source),
		)
		rendered, err := l.renderer.RenderWidgets(
			r.Context(),
			"sidebar",
			nil,
			features,
			capabilities,
			preferences.HiddenPluginWidgets,
		)
		stop()
		if err != nil {
			return pluginData{}, err
		}
		sidebarWidgets = webview.Widgets(rendered, "sidebar", "", r.URL.RequestURI())
	}

	modules, err := pluginModulesJSON(l.pluginManager, "/plugins")
	if err != nil {
		return pluginData{}, err
	}

	return pluginData{
		features:          features,
		widgetPreferences: webview.PluginWidgetPreferences(loadedPlugins, preferences.HiddenPluginWidgets),
		modules:           modules,
		stylesVersion:     pluginbrowser.PresentationStylesVersion(l.pluginManager),
		editorInserts:     editorInserts,
		sidebarWidgets:    sidebarWidgets,
		settingsLinks:     pluginSettingsLinks(loadedPlugins, pluginSettingsIconCatalog(l.renderer)),
	}, nil
}

const defaultPluginSettingsIcon = "puzzle-lucide"

// pluginSettingsIconValidator reports whether an icon is available to the current renderer.
type pluginSettingsIconValidator interface {
	IsIcon(string) bool
}

// pluginSettingsIconCatalog returns the active renderer icon catalog when available.
func pluginSettingsIconCatalog(renderer *md.Renderer) pluginSettingsIconValidator {
	if renderer == nil {
		return nil
	}
	return renderer.IconCatalog()
}

// pluginSettingsLinks returns installed plugins that expose administrator-managed configuration.
func pluginSettingsLinks(items []plugin.LoadedPlugin, icons pluginSettingsIconValidator) []webview.PluginSettingsLink {
	links := make([]webview.PluginSettingsLink, 0)
	for _, item := range items {
		if !pluginExposesSettings(item) {
			continue
		}

		icon := defaultPluginSettingsIcon
		if item.Manifest.Icon != "" && icons != nil && icons.IsIcon(item.Manifest.Icon) {
			icon = item.Manifest.Icon
		}

		links = append(links, webview.PluginSettingsLink{
			ID:      item.Manifest.ID,
			Name:    item.Manifest.Name,
			Icon:    icon,
			Section: "plugin:" + item.Manifest.ID,
		})
	}
	sort.Slice(links, func(i, j int) bool {
		return strings.ToLower(links[i].Name) < strings.ToLower(links[j].Name)
	})
	return links
}

// pluginExposesSettings reports whether a plugin contributes administrator-managed configuration.
func pluginExposesSettings(item plugin.LoadedPlugin) bool {
	for _, module := range item.Manifest.Modules {
		if module.Type == "settings" || module.Type == "admin-resource" || module.Type == "admin-action" {
			return true
		}
	}
	return false
}

// effectiveTypographySize resolves the user preference against the application default.
func effectiveTypographySize(preferences domain.UserPreferences, settings domain.ApplicationSettings) string {
	typographySize := preferences.TypographySize
	if typographySize == "" {
		typographySize = settings.Rendering.DefaultTypographySize
	}
	if !domain.ValidTypographySize(typographySize) {
		return domain.DefaultTypographySize
	}

	return typographySize
}

// selectedTheme resolves a stored theme preference to an available theme title.
func selectedTheme(views *webview.Views, preference string) string {
	if selected, ok := themes.Find(views.Themes(), preference); ok {
		return selected.Title
	}

	return themes.DefaultTheme
}

// pluginModulesJSON serializes active browser modules for direct page embedding.
func pluginModulesJSON(manager *plugin.Manager, prefix string) (template.JS, error) {
	data, err := json.Marshal(pluginbrowser.Catalog(prefix, manager))
	if err != nil {
		return "", err
	}

	return template.JS(data), nil
}

// activeNavigationSlug extracts the current page slug from browser page and editor routes.
func activeNavigationSlug(requestPath string) string {
	for _, prefix := range []string{"/pages/", "/edit/"} {
		slug, ok := strings.CutPrefix(requestPath, prefix)
		if !ok {
			continue
		}

		slug = strings.Trim(slug, "/")
		if slug == "new" {
			return ""
		}
		return slug
	}
	return ""
}
