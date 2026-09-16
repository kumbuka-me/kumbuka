package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
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

// publicViewData builds shared data for unauthenticated setup and login pages.
func publicViewData(views *Views, title string) (ViewData, error) {
	preferences := domain.DefaultUserPreferences()
	activeTheme := themes.DefaultTheme
	preferences.Theme = activeTheme
	themeData, err := json.Marshal(views.themes)
	if err != nil {
		return ViewData{}, err
	}

	return ViewData{
		Title:         title,
		Preferences:   preferences,
		Version:       views.version,
		AssetVersion:  views.assetVersion,
		Commit:        views.commit,
		Runtime:       views.runtime,
		ThemeData:     template.JS(themeData),
		PluginModules: template.JS("[]"),
		Themes:        views.themes,
		ActiveTheme:   activeTheme,
	}, nil
}

// ViewDataLoader assembles the shared data required by authenticated HTML views.
type ViewDataLoader struct {
	// preferenceUseCases loads per-user presentation preferences.
	preferenceUseCases preferenceService
	// navigationUseCases loads page paths and persisted navigation icons.
	navigationUseCases navigationService
	// catalogUseCases provides user-specific page collections for sidebar widgets.
	catalogUseCases sidebarCatalogService
	// settingsUseCases loads application-wide rendering and content settings.
	settingsUseCases settingsService
	// savedSearchUseCases loads the current user's saved searches.
	savedSearchUseCases savedSearchReader
	// notificationUseCases loads recent notifications and unread counts.
	notificationUseCases notificationReader
	// accessUseCases filters pages according to the current user's permissions.
	accessUseCases pageAccessReader
	// pluginManager exposes active plugin metadata and browser contributions.
	pluginManager *plugin.Manager
	// renderer renders plugin-owned sidebar widgets.
	renderer *md.Renderer
}

// NewViewDataLoader constructs the shared authenticated view-data loader.
func NewViewDataLoader(
	preferences preferenceService,
	navigation navigationService,
	catalog sidebarCatalogService,
	settings settingsService,
	savedSearches savedSearchReader,
	notifications notificationReader,
	access pageAccessReader,
	renderer *md.Renderer,
) *ViewDataLoader {
	var plugins *plugin.Manager
	if renderer != nil {
		plugins = renderer.PluginManager()
	}

	return &ViewDataLoader{
		preferenceUseCases:   preferences,
		navigationUseCases:   navigation,
		catalogUseCases:      catalog,
		settingsUseCases:     settings,
		savedSearchUseCases:  savedSearches,
		notificationUseCases: notifications,
		accessUseCases:       access,
		pluginManager:        plugins,
		renderer:             renderer,
	}
}

// pluginViewData groups plugin-owned contributions shared by authenticated templates.
type pluginViewData struct {
	// features contains enabled plugin and setting flags for browser decisions.
	features map[string]bool
	// widgetPreferences contains user-facing visibility controls for active widgets.
	widgetPreferences []pluginWidgetPreferenceView
	// modules contains the serialized browser-module catalog.
	modules template.JS
	// stylesVersion fingerprints active plugin presentation styles.
	stylesVersion string
	// editorInserts contains plugin-owned editor actions.
	editorInserts []plugin.EditorInsertContribution
	// sidebarWidgets contains rendered plugin widgets for the sidebar surface.
	sidebarWidgets []pluginWidgetView
}

// Load builds the common template data used by every browser page.
func (l *ViewDataLoader) Load(r *http.Request, views *Views, title string) (ViewData, error) {
	user, _ := auth.User(r)

	stop := measurePageStage(r.Context(), "view_preferences")
	preferences, err := l.preferenceUseCases.Preferences(r.Context(), user.ID)
	stop()
	if err != nil {
		return ViewData{}, err
	}

	pageNavigation, err := l.loadNavigation(r, user, preferences)
	if err != nil {
		return ViewData{}, err
	}

	stop = measurePageStage(r.Context(), "view_application_settings")
	applicationSettings, err := l.settingsUseCases.ApplicationSettings(r.Context())
	stop()
	if err != nil {
		return ViewData{}, err
	}

	typographySize := effectiveTypographySize(preferences, applicationSettings)
	activeTheme := selectedTheme(views, preferences.Theme)
	preferences.Theme = activeTheme

	stop = measurePageStage(r.Context(), "view_theme_data")
	themeData, err := json.Marshal(views.themes)
	stop()
	if err != nil {
		return ViewData{}, err
	}

	stop = measurePageStage(r.Context(), "view_saved_searches")
	savedSearches, err := l.savedSearchUseCases.SavedSearches(r.Context(), user.ID)
	stop()
	if err != nil {
		return ViewData{}, err
	}

	stop = measurePageStage(r.Context(), "view_notifications")
	notifications, unreadNotifications, err := l.notificationUseCases.Notifications(r.Context(), user.ID, 8)
	stop()
	if err != nil {
		return ViewData{}, err
	}

	plugins, err := l.loadPluginViewData(r, user, preferences)
	if err != nil {
		return ViewData{}, err
	}

	return ViewData{
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
		CanEdit:                 user.Role == "admin" || user.Role == "editor",
		PageContentLanguage:     applicationSettings.ContentLanguage,
	}, nil
}

// loadNavigation builds the accessible sidebar navigation for non-administration pages.
func (l *ViewDataLoader) loadNavigation(
	r *http.Request,
	user domain.User,
	preferences domain.UserPreferences,
) ([]navigation.Node, error) {
	if strings.HasPrefix(r.URL.Path, "/admin") {
		return nil, nil
	}

	stop := measurePageStage(r.Context(), "view_navigation_pages")
	pages, err := l.navigationUseCases.NavigationPages(r.Context())
	stop()
	if err != nil {
		return nil, err
	}

	stop = measurePageStage(r.Context(), "view_navigation_filter")
	pages, err = l.accessUseCases.FilterPages(r.Context(), user, pages)
	stop()
	if err != nil {
		return nil, err
	}

	stop = measurePageStage(r.Context(), "view_navigation_icons")
	navigationIcons, err := l.navigationUseCases.NavigationIcons(r.Context())
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

// loadPluginViewData resolves active plugin flags, browser assets, editor actions, and sidebar widgets.
func (l *ViewDataLoader) loadPluginViewData(
	r *http.Request,
	user domain.User,
	preferences domain.UserPreferences,
) (pluginViewData, error) {
	stop := measurePageStage(r.Context(), "view_plugin_features")
	features := make(map[string]bool)
	var editorInserts []plugin.EditorInsertContribution
	var loadedPlugins []plugin.LoadedPlugin
	if l.pluginManager != nil {
		editorInserts = l.pluginManager.EditorInserts()
		loadedPlugins = l.pluginManager.Plugins()
		for _, item := range loadedPlugins {
			features[item.Manifest.ID] = item.Enabled
			if !item.Enabled {
				continue
			}
			for key, enabled := range item.Settings {
				features[item.Manifest.ID+"."+key] = enabled
			}
		}
	}
	stop()

	var sidebarWidgets []pluginWidgetView
	if !strings.HasPrefix(r.URL.Path, "/admin") && l.renderer != nil {
		stop = measurePageStage(r.Context(), "view_sidebar_widgets")
		source := personalWidgetSource{catalog: l.catalogUseCases, access: l.accessUseCases, user: user}
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
			return pluginViewData{}, err
		}
		sidebarWidgets = widgetViews(rendered)
	}

	modules, err := pluginModulesJSON(l.pluginManager, "/plugins")
	if err != nil {
		return pluginViewData{}, err
	}

	return pluginViewData{
		features:          features,
		widgetPreferences: pluginWidgetPreferences(loadedPlugins, preferences.HiddenPluginWidgets),
		modules:           modules,
		stylesVersion:     pluginbrowser.PresentationStylesVersion(l.pluginManager),
		editorInserts:     editorInserts,
		sidebarWidgets:    sidebarWidgets,
	}, nil
}

// effectiveTypographySize resolves the user's typography preference against the application default.
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
