package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/pluginbrowser"
	"github.com/kumbuka-me/kumbuka/internal/plugincap"
	"github.com/kumbuka-me/kumbuka/themes"
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
	preferenceUseCases   preferenceService
	navigationUseCases   navigationService
	catalogUseCases      sidebarCatalogService
	settingsUseCases     settingsService
	savedSearchUseCases  savedSearchReader
	notificationUseCases notificationReader
	accessUseCases       pageAccessReader
	pluginManager        *plugin.Manager
	renderer             *md.Renderer
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

// Load builds the common template data used by every browser page.
func (l *ViewDataLoader) Load(r *http.Request, views *Views, title string) (ViewData, error) {
	user, _ := auth.User(r)

	stop := measurePageStage(r.Context(), "view_preferences")
	preferences, err := l.preferenceUseCases.Preferences(r.Context(), user.ID)
	stop()
	if err != nil {
		return ViewData{}, err
	}

	var pageNavigation []navigation.Node

	if !strings.HasPrefix(r.URL.Path, "/admin") {
		stop = measurePageStage(r.Context(), "view_navigation_pages")
		pages, err := l.navigationUseCases.NavigationPages(r.Context())
		stop()
		if err != nil {
			return ViewData{}, err
		}

		stop = measurePageStage(r.Context(), "view_navigation_filter")
		pages, err = l.accessUseCases.FilterPages(r.Context(), user, pages)
		stop()
		if err != nil {
			return ViewData{}, err
		}

		stop = measurePageStage(r.Context(), "view_navigation_icons")
		navigationIcons, err := l.navigationUseCases.NavigationIcons(r.Context())
		stop()
		if err != nil {
			return ViewData{}, err
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
		pageNavigation = navigation.Build(navigationPages, navigation.Options{
			ActiveSlug:     activeNavigationSlug(r.URL.Path),
			Expanded:       expanded,
			ShowPageCounts: preferences.ShowNavigationPageCounts,
			Icons:          navigationIcons,
		})
		stop()

	}

	stop = measurePageStage(r.Context(), "view_application_settings")
	applicationSettings, err := l.settingsUseCases.ApplicationSettings(r.Context())
	stop()
	if err != nil {
		return ViewData{}, err
	}

	typographySize := preferences.TypographySize
	if typographySize == "" {
		typographySize = applicationSettings.Rendering.DefaultTypographySize
	}
	if !domain.ValidTypographySize(typographySize) {
		typographySize = domain.DefaultTypographySize
	}

	activeTheme := themes.DefaultTheme

	if selected, ok := themes.Find(views.themes, preferences.Theme); ok {
		activeTheme = selected.Title
	}

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

	stop = measurePageStage(r.Context(), "view_plugin_features")
	pluginFeatures := make(map[string]bool)
	var editorInserts []plugin.EditorInsertContribution
	var loadedPlugins []plugin.LoadedPlugin
	if l.pluginManager != nil {
		editorInserts = l.pluginManager.EditorInserts()
		loadedPlugins = l.pluginManager.Plugins()
		for _, item := range loadedPlugins {
			pluginFeatures[item.Manifest.ID] = item.Enabled
			if !item.Enabled {
				continue
			}
			for key, enabled := range item.Settings {
				pluginFeatures[item.Manifest.ID+"."+key] = enabled
			}
		}
	}
	stop()

	var sidebarWidgets []pluginWidgetView
	if !strings.HasPrefix(r.URL.Path, "/admin") && l.renderer != nil {
		source := personalWidgetSource{catalog: l.catalogUseCases, access: l.accessUseCases, user: user}
		capabilities := plugincap.MergeCapabilities(
			plugincap.Capabilities(nil, nil, l.renderer.IconCatalog()),
			plugincap.PageListCapabilities(source),
		)
		rendered, renderErr := l.renderer.RenderWidgets(r.Context(), "sidebar", nil, pluginFeatures, capabilities, preferences.HiddenPluginWidgets)
		if renderErr != nil {
			return ViewData{}, renderErr
		}
		sidebarWidgets = widgetViews(rendered)
	}

	pluginModules, err := pluginModulesJSON(l.pluginManager, "/plugins")
	if err != nil {
		return ViewData{}, err
	}
	pluginStylesVersion := pluginbrowser.PresentationStylesVersion(l.pluginManager)

	return ViewData{
		Title:                   title,
		User:                    user,
		Preferences:             preferences,
		TypographySize:          typographySize,
		Navigation:              pageNavigation,
		NewPageParent:           activeNavigationSlug(r.URL.Path),
		SidebarWidgets:          sidebarWidgets,
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
		PluginFeatures:          pluginFeatures,
		PluginWidgetPreferences: pluginWidgetPreferences(loadedPlugins, preferences.HiddenPluginWidgets),
		PluginModules:           pluginModules,
		PluginStylesVersion:     pluginStylesVersion,
		EditorInserts:           editorInserts,
		CanEdit:                 user.Role == "admin" || user.Role == "editor",
		PageContentLanguage:     applicationSettings.ContentLanguage,
	}, nil
}

// pluginModulesJSON serializes active browser modules for direct page embedding.
func pluginModulesJSON(manager *plugin.Manager, prefix string) (template.JS, error) {
	data, err := json.Marshal(pluginbrowser.Catalog(prefix, manager))
	if err != nil {
		return "", err
	}

	return template.JS(data), nil
}

// viewData loads shared authenticated view data through the handler's narrow dependency.
func viewData(r *http.Request, loader viewDataService, views *Views, title string) (ViewData, error) {
	return loader.Load(r, views, title)
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
