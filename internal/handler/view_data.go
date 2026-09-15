package handler

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugin"
	"github.com/kumbuka-me/kumbuka/internal/pluginbrowser"
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
	plugins *plugin.Manager,
) *ViewDataLoader {
	return &ViewDataLoader{
		preferenceUseCases:   preferences,
		navigationUseCases:   navigation,
		catalogUseCases:      catalog,
		settingsUseCases:     settings,
		savedSearchUseCases:  savedSearches,
		notificationUseCases: notifications,
		accessUseCases:       access,
		pluginManager:        plugins,
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
	var sidebarPinned []domain.Page
	var sidebarRecent []domain.Page

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

		if preferences.ShowPinnedPages {
			stop = measurePageStage(r.Context(), "view_sidebar_pinned")
			sidebarPinned, err = l.catalogUseCases.Favorites(r.Context(), user.ID)
			stop()
			if err != nil {
				return ViewData{}, err
			}
			stop = measurePageStage(r.Context(), "view_sidebar_pinned_filter")
			sidebarPinned, err = l.accessUseCases.FilterPages(r.Context(), user, sidebarPinned)
			stop()
			if err != nil {
				return ViewData{}, err
			}
		}
		if preferences.ShowRecentlyViewed {
			stop = measurePageStage(r.Context(), "view_sidebar_recent")
			sidebarRecent, err = l.catalogUseCases.RecentViewed(r.Context(), user.ID, 8)
			stop()
			if err != nil {
				return ViewData{}, err
			}
			stop = measurePageStage(r.Context(), "view_sidebar_recent_filter")
			sidebarRecent, err = l.accessUseCases.FilterPages(r.Context(), user, sidebarRecent)
			stop()
			if err != nil {
				return ViewData{}, err
			}

			sidebarRecent = pagesWithout(sidebarRecent, sidebarPinned, 5)
		}
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
	if l.pluginManager != nil {
		editorInserts = l.pluginManager.EditorInserts()
		for _, item := range l.pluginManager.Plugins() {
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

	pluginModules, err := pluginModulesJSON(l.pluginManager, "/plugins")
	if err != nil {
		return ViewData{}, err
	}
	pluginStylesVersion := pluginbrowser.PresentationStylesVersion(l.pluginManager)

	return ViewData{
		Title:               title,
		User:                user,
		Preferences:         preferences,
		TypographySize:      typographySize,
		Navigation:          pageNavigation,
		NewPageParent:       activeNavigationSlug(r.URL.Path),
		SidebarPinned:       sidebarPinned,
		SidebarRecent:       sidebarRecent,
		SavedSearches:       savedSearches,
		Notifications:       notifications,
		UnreadNotifications: unreadNotifications,
		PageStatuses:        domain.PageStatuses(),
		Version:             views.version,
		AssetVersion:        views.assetVersion,
		Commit:              views.commit,
		Runtime:             views.runtime,
		ThemeData:           template.JS(themeData),
		Themes:              views.themes,
		ActiveTheme:         activeTheme,
		ApplicationSettings: applicationSettings,
		PluginFeatures:      pluginFeatures,
		PluginModules:       pluginModules,
		PluginStylesVersion: pluginStylesVersion,
		EditorInserts:       editorInserts,
		CanEdit:             user.Role == "admin" || user.Role == "editor",
		PageContentLanguage: applicationSettings.ContentLanguage,
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

// pagesWithout returns up to limit pages excluding any page present in excluded.
func pagesWithout(pages, excluded []domain.Page, limit int) []domain.Page {
	if limit <= 0 {
		return nil
	}

	excludedIDs := make(map[int64]bool, len(excluded))

	for _, page := range excluded {
		excludedIDs[page.ID] = true
	}

	result := make([]domain.Page, 0, min(limit, len(pages)))

	for _, page := range pages {
		if excludedIDs[page.ID] {
			continue
		}

		result = append(result, page)
		if len(result) == limit {
			break
		}
	}

	return result
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
