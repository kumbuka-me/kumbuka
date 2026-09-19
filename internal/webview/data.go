package webview

import (
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/pluginbrowser"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

// preferenceReader loads the current user's presentation preferences.
type preferenceReader interface {
	Preferences(context.Context, int64) (domain.UserPreferences, error)
}

// navigationReader loads page paths and persisted navigation icons.
type navigationReader interface {
	NavigationPages(context.Context) ([]domain.Page, error)
	NavigationIcons(context.Context) (map[string]string, error)
}

// sidebarCatalogReader loads personal page lists used by sidebar widgets.
type sidebarCatalogReader interface {
	Favorites(context.Context, int64) ([]domain.Page, error)
	RecentViewed(context.Context, int64, int) ([]domain.Page, error)
}

// settingsReader loads application-wide presentation settings.
type settingsReader interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
}

// savedSearchReader loads the current user's saved searches.
type savedSearchReader interface {
	SavedSearches(context.Context, int64) ([]domain.SavedSearch, error)
}

// notificationReader loads recent notifications and unread counts.
type notificationReader interface {
	Notifications(context.Context, int64, int) ([]domain.Notification, int, error)
}

// accessReader filters page collections for the current viewer.
type accessReader interface {
	FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error)
}

// PublicData builds shared data for unauthenticated setup and login pages.
func (v *Views) PublicData(title string) (Data, error) {
	preferences := domain.DefaultUserPreferences()
	activeTheme := themes.DefaultTheme
	preferences.Theme = activeTheme
	themeData, err := json.Marshal(v.themes)
	if err != nil {
		return Data{}, err
	}

	return Data{
		Title:         title,
		Preferences:   preferences,
		Version:       v.version,
		AssetVersion:  v.assetVersion,
		Commit:        v.commit,
		Runtime:       v.runtime,
		ThemeData:     template.JS(themeData),
		PluginModules: template.JS("[]"),
		Themes:        v.themes,
		ActiveTheme:   activeTheme,
	}, nil
}

// PublicPluginData builds unauthenticated view data with active browser plugin presentation assets.
func (v *Views) PublicPluginData(title string, manager *plugin.Manager) (Data, error) {
	data, err := v.PublicData(title)
	if err != nil {
		return Data{}, err
	}

	data.PluginModules, err = pluginModulesJSON(manager, "/plugins")
	if err != nil {
		return Data{}, err
	}
	data.PluginStylesVersion = pluginbrowser.PresentationStylesVersion(manager)

	return data, nil
}

// Loader assembles the shared data required by authenticated HTML views.
type Loader struct {
	// preferences loads per-user presentation preferences.
	preferences preferenceReader
	// navigation loads page paths and persisted navigation icons.
	navigation navigationReader
	// catalog loads personal page lists used by sidebar widgets.
	catalog sidebarCatalogReader
	// settings loads application-wide presentation settings.
	settings settingsReader
	// savedSearches loads the current user's saved searches.
	savedSearches savedSearchReader
	// notifications loads recent notifications and unread counts.
	notifications notificationReader
	// access filters page collections for the current viewer.
	access accessReader
	// pluginManager exposes active browser and editor contributions.
	pluginManager *plugin.Manager
	// renderer renders plugin-owned sidebar widgets.
	renderer *md.Renderer
}

// NewLoader constructs the shared authenticated view-data loader.
func NewLoader(
	preferences preferenceReader,
	navigation navigationReader,
	catalog sidebarCatalogReader,
	settings settingsReader,
	// savedSearches loads the current user's saved searches.
	savedSearches savedSearchReader,
	// notifications loads recent notifications and unread counts.
	notifications notificationReader,
	access accessReader,
	renderer *md.Renderer,
) *Loader {
	var plugins *plugin.Manager
	if renderer != nil {
		plugins = renderer.PluginManager()
	}

	return &Loader{
		preferences:   preferences,
		navigation:    navigation,
		catalog:       catalog,
		settings:      settings,
		savedSearches: savedSearches,
		notifications: notifications,
		access:        access,
		pluginManager: plugins,
		renderer:      renderer,
	}
}

// pluginData groups plugin-owned contributions shared by authenticated templates.
type pluginData struct {
	// features contains enabled plugin and setting flags.
	features map[string]bool
	// widgetPreferences contains user-facing visibility controls.
	widgetPreferences []WidgetPreference
	// modules contains the serialized browser-module catalog.
	modules template.JS
	// stylesVersion fingerprints active plugin presentation styles.
	stylesVersion string
	// editorInserts contains active editor actions.
	editorInserts []plugin.EditorInsertContribution
	// sidebarWidgets contains rendered sidebar widget models.
	sidebarWidgets []Widget
	// settingsLinks contains installed plugins with administrator configuration.
	settingsLinks []PluginSettingsLink
}

// Load builds the common template data used by every authenticated browser page.
func (l *Loader) Load(r *http.Request, views *Views, title string) (Data, error) {
	user, _ := auth.User(r)

	stop := measurePageStage(r.Context(), "view_preferences")
	preferences, err := l.preferences.Preferences(r.Context(), user.ID)
	stop()
	if err != nil {
		return Data{}, err
	}

	pageNavigation, err := l.loadNavigation(r, user, preferences)
	if err != nil {
		return Data{}, err
	}

	stop = measurePageStage(r.Context(), "view_application_settings")
	applicationSettings, err := l.settings.ApplicationSettings(r.Context())
	stop()
	if err != nil {
		return Data{}, err
	}

	typographySize := effectiveTypographySize(preferences, applicationSettings)
	activeTheme := selectedTheme(views, preferences.Theme)
	preferences.Theme = activeTheme

	stop = measurePageStage(r.Context(), "view_theme_data")
	themeData, err := json.Marshal(views.themes)
	stop()
	if err != nil {
		return Data{}, err
	}

	stop = measurePageStage(r.Context(), "view_saved_searches")
	savedSearches, err := l.savedSearches.SavedSearches(r.Context(), user.ID)
	stop()
	if err != nil {
		return Data{}, err
	}

	stop = measurePageStage(r.Context(), "view_notifications")
	notifications, unreadNotifications, err := l.notifications.Notifications(r.Context(), user.ID, 8)
	stop()
	if err != nil {
		return Data{}, err
	}

	plugins, err := l.loadPluginData(r, user, preferences)
	if err != nil {
		return Data{}, err
	}

	return Data{
		Title:                   title,
		User:                    user,
		Preferences:             preferences,
		TypographySize:          typographySize,
		Navigation:              pageNavigation,
		NewPageParent:           activeNavigationSlug(r.URL.Path),
		SidebarWidgets:          plugins.sidebarWidgets,
		SavedSearches:           savedSearches,
		Notifications:           notifications,
		UnreadNotifications:     unreadNotifications,
		PageStatuses:            domain.PageStatuses(),
		Version:                 views.version,
		AssetVersion:            views.assetVersion,
		Commit:                  views.commit,
		Runtime:                 views.runtime,
		ThemeData:               template.JS(themeData),
		Themes:                  views.themes,
		ActiveTheme:             activeTheme,
		ApplicationSettings:     applicationSettings,
		PluginFeatures:          plugins.features,
		PluginWidgetPreferences: plugins.widgetPreferences,
		PluginModules:           plugins.modules,
		PluginStylesVersion:     plugins.stylesVersion,
		EditorInserts:           plugins.editorInserts,
		PluginSettingsLinks:     plugins.settingsLinks,
		CanEdit:                 user.CanEditContent(),
		PageContentLanguage:     applicationSettings.ContentLanguage,
	}, nil
}

// loadNavigation builds the accessible sidebar navigation for non-administration pages.
func (l *Loader) loadNavigation(
	r *http.Request,
	user domain.User,
	preferences domain.UserPreferences,
) ([]navigation.Node, error) {
	if strings.HasPrefix(r.URL.Path, "/admin") {
		return nil, nil
	}

	stop := measurePageStage(r.Context(), "view_navigation_pages")
	pages, err := l.navigation.NavigationPages(r.Context())
	stop()
	if err != nil {
		return nil, err
	}

	stop = measurePageStage(r.Context(), "view_navigation_filter")
	pages, err = l.access.FilterPages(r.Context(), user, pages)
	stop()
	if err != nil {
		return nil, err
	}

	stop = measurePageStage(r.Context(), "view_navigation_icons")
	navigationIcons, err := l.navigation.NavigationIcons(r.Context())
	stop()
	if err != nil {
		return nil, err
	}

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

	stop = measurePageStage(r.Context(), "view_navigation_build")
	result := navigation.Build(navigationPages, navigation.Options{
		ActiveSlug:     activeNavigationSlug(r.URL.Path),
		Expanded:       expanded,
		ShowPageCounts: preferences.ShowNavigationPageCounts,
		Icons:          navigationIcons,
	})
	stop()

	return result, nil
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
func (l *Loader) loadPluginData(
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

	var sidebarWidgets []Widget
	if !strings.HasPrefix(r.URL.Path, "/admin") && l.renderer != nil {
		stop = measurePageStage(r.Context(), "view_sidebar_widgets")
		source := personalWidgetSource{catalog: l.catalog, access: l.access, user: user}
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
		sidebarWidgets = Widgets(rendered, "sidebar", "", r.URL.RequestURI())
	}

	modules, err := pluginModulesJSON(l.pluginManager, "/plugins")
	if err != nil {
		return pluginData{}, err
	}

	return pluginData{
		features:          features,
		widgetPreferences: pluginWidgetPreferences(loadedPlugins, preferences.HiddenPluginWidgets),
		modules:           modules,
		stylesVersion:     pluginbrowser.PresentationStylesVersion(l.pluginManager),
		editorInserts:     editorInserts,
		sidebarWidgets:    sidebarWidgets,
		settingsLinks:     pluginSettingsLinks(loadedPlugins),
	}, nil
}

// pluginSettingsLinks returns installed plugins that expose boolean settings or structured resources.
func pluginSettingsLinks(items []plugin.LoadedPlugin) []PluginSettingsLink {
	links := make([]PluginSettingsLink, 0)
	for _, item := range items {
		if !pluginExposesSettings(item) {
			continue
		}
		links = append(links, PluginSettingsLink{
			ID:      item.Manifest.ID,
			Name:    item.Manifest.Name,
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
		if module.Type == "settings" || module.Type == "admin-resource" {
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
func selectedTheme(views *Views, preference string) string {
	if selected, ok := themes.Find(views.themes, preference); ok {
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
