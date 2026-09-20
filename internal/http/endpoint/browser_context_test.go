package endpoint

import (
	"context"
	"errors"
	"github.com/kumbuka-me/kumbuka/internal/application/viewer"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/web"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/kumbuka-me/sdk/pluginpackage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrowserContextLoad(t *testing.T) {
	t.Parallel()

	t.Run("loads authenticated navigation and shared chrome data", func(t *testing.T) {
		t.Parallel()

		user := domain.User{ID: 42, Username: "editor", Role: "editor", Enabled: true}
		preferences := domain.DefaultUserPreferences()
		preferences.Theme = "Dark"
		preferences.TypographySize = ""
		preferences.ShowNavigationPageCounts = true
		preferences.ExpandedNavigation = []string{"platforms"}

		browserContext := newTestBrowserContext(
			browserContextPreferenceStub{preferences: preferences},
			browserContextNavigationStub{
				pages: []domain.Page{
					{ID: 1, Slug: "platforms", Title: "Platforms"},
					{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"},
					{ID: 99, Slug: "restricted", Title: "Restricted"},
				},
				icons: map[string]string{"platforms": "folder-lucide"},
			},
			browserContextCatalogStub{
				favorites: []domain.Page{{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"}},
				recent: []domain.Page{
					{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"},
					{ID: 4, Slug: "platforms/nomad", Title: "Nomad"},
					{ID: 5, Slug: "platforms/consul", Title: "Consul"},
				},
			},
			browserContextSettingsStub{settings: domain.ApplicationSettings{
				ContentLanguage: "de-CH",
				Rendering: domain.RenderingSettings{
					DefaultTypographySize: domain.TypographySizeLarge,
				},
			}},
			browserContextSavedSearchStub{searches: []domain.SavedSearch{{ID: 7, Name: "Production", Query: "tag:prod"}}},
			browserContextNotificationStub{
				notifications: []domain.Notification{{ID: 8, Title: "Mention"}},
				unread:        1,
			},
			browserContextAccessStub{filter: func(pages []domain.Page) []domain.Page {
				filtered := make([]domain.Page, 0, len(pages))
				for _, page := range pages {
					if page.ID != 99 {
						filtered = append(filtered, page)
					}
				}
				return filtered
			}},
			nil,
		)
		views, err := webview.New(web.Assets, testViewsLogger(), "v1.2.3", "abc123", []themes.Theme{
			{Title: "Light", ColorScheme: "light"}, {Title: "Dark", ColorScheme: "dark"},
		}, webview.RuntimeInfo{PublicURL: "https://kumbuka.example.test"})
		require.NoError(t, err)

		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/pages/platforms/kubernetes", nil),
			user,
		)

		data, err := browserContext.Load(request, views, "Kubernetes")

		require.NoError(t, err)
		assert.Equal(t, "Kubernetes", data.Title)
		assert.Equal(t, user, data.User)
		assert.True(t, data.CanEdit)
		assert.Equal(t, domain.TypographySizeLarge, data.TypographySize)
		assert.Equal(t, "Dark", data.ActiveTheme)
		assert.Equal(t, "Dark", data.Preferences.Theme)
		assert.Equal(t, "platforms/kubernetes", data.NewPageParent)
		assert.Equal(t, "de-CH", data.ApplicationSettings.ContentLanguage)
		assert.Equal(t, "v1.2.3", data.Version)
		assert.Equal(t, "abc123", data.Commit)
		assert.Equal(t, views.AssetVersion(), data.AssetVersion)
		assert.Len(t, data.SavedSearches, 1)
		assert.Len(t, data.Notifications, 1)
		assert.Equal(t, 1, data.UnreadNotifications)

		require.Len(t, data.Navigation, 1)
		assert.Equal(t, "platforms", data.Navigation[0].Slug)
		assert.Equal(t, "folder-lucide", data.Navigation[0].Icon)
		assert.True(t, data.Navigation[0].Open)
		assert.True(t, data.Navigation[0].ShowPageCount)
		require.Len(t, data.Navigation[0].Children, 1)
		assert.Equal(t, "platforms/kubernetes", data.Navigation[0].Children[0].Slug)
		assert.True(t, data.Navigation[0].Children[0].Active)

		assert.Empty(t, data.SidebarWidgets)
	})

	t.Run("skips navigation dependencies for admin pages", func(t *testing.T) {
		t.Parallel()

		preferences := domain.DefaultUserPreferences()
		preferences.Theme = "missing-theme"
		preferences.TypographySize = "invalid-size"
		user := domain.User{ID: 11, Username: "viewer", Role: "viewer", Enabled: true}
		browserContext := newTestBrowserContext(
			browserContextPreferenceStub{preferences: preferences},
			nil,
			nil,
			browserContextSettingsStub{settings: domain.ApplicationSettings{}},
			browserContextSavedSearchStub{},
			browserContextNotificationStub{},
			nil,
			nil,
		)
		views, err := webview.New(web.Assets, testViewsLogger(), "", "", []themes.Theme{{Title: "Dark", ColorScheme: "dark"}}, webview.RuntimeInfo{})
		require.NoError(t, err)
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/admin", nil), user)

		data, err := browserContext.Load(request, views, "Administration")

		require.NoError(t, err)
		assert.Empty(t, data.Navigation)
		assert.Empty(t, data.SidebarWidgets)
		assert.Equal(t, themes.DefaultTheme, data.ActiveTheme)
		assert.Equal(t, domain.DefaultTypographySize, data.TypographySize)
		assert.False(t, data.CanEdit)
	})

	t.Run("returns preference errors", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("load preferences")
		browserContext := newTestBrowserContext(
			browserContextPreferenceStub{err: wantErr},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)

		_, err := browserContext.Load(
			httptest.NewRequest(http.MethodGet, "/admin", nil),
			&webview.Views{},
			"Administration",
		)

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns settings errors", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("load settings")
		browserContext := newTestBrowserContext(
			browserContextPreferenceStub{preferences: domain.DefaultUserPreferences()},
			nil,
			nil,
			browserContextSettingsStub{err: wantErr},
			nil,
			nil,
			nil,
			nil,
		)

		_, err := browserContext.Load(
			httptest.NewRequest(http.MethodGet, "/admin", nil),
			&webview.Views{},
			"Administration",
		)

		assert.ErrorIs(t, err, wantErr)
	})
}

func TestActiveNavigationSlug(t *testing.T) {
	t.Parallel()

	t.Run("extracts page route", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "applications/identity/keycloak", activeNavigationSlug("/pages/applications/identity/keycloak"))
	})

	t.Run("extracts edit route", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "platforms/kubernetes", activeNavigationSlug("/edit/platforms/kubernetes"))
	})

	t.Run("trims trailing slash", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "platforms/kubernetes", activeNavigationSlug("/pages/platforms/kubernetes/"))
	})

	t.Run("ignores new page route", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, activeNavigationSlug("/pages/new"))
	})

	t.Run("ignores unrelated route", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, activeNavigationSlug("/settings"))
	})
}

type browserContextPreferenceStub struct {
	preferences domain.UserPreferences
	err         error
}

func (s browserContextPreferenceStub) Preferences(context.Context, int64) (domain.UserPreferences, error) {
	return s.preferences, s.err
}

func (browserContextPreferenceStub) SavePreferences(context.Context, int64, domain.UserPreferences) error {
	return nil
}

func (browserContextPreferenceStub) SetShowPageContents(context.Context, int64, bool) error {
	return nil
}

func (browserContextPreferenceStub) SetExpandedNavigation(context.Context, int64, []string) error {
	return nil
}

func (browserContextPreferenceStub) SetSidebarWidth(context.Context, int64, int) error {
	return nil
}

type browserContextNavigationStub struct {
	pages []domain.Page
	icons map[string]string
	err   error
}

func (s browserContextNavigationStub) NavigationPages(context.Context) ([]domain.Page, error) {
	return s.pages, s.err
}

func (browserContextNavigationStub) NavigationItems(context.Context) ([]domain.NavigationItem, error) {
	return nil, nil
}

func (s browserContextNavigationStub) NavigationIcons(context.Context) (map[string]string, error) {
	return s.icons, s.err
}

func (browserContextNavigationStub) SetNavigationIcon(context.Context, string, string) error {
	return nil
}

type browserContextCatalogStub struct {
	favorites []domain.Page
	recent    []domain.Page
}

func (s browserContextCatalogStub) Favorites(context.Context, int64) ([]domain.Page, error) {
	return s.favorites, nil
}

func (s browserContextCatalogStub) RecentViewed(context.Context, int64, int) ([]domain.Page, error) {
	return s.recent, nil
}

type browserContextSettingsStub struct {
	settings domain.ApplicationSettings
	err      error
}

func (s browserContextSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, s.err
}

type browserContextSavedSearchStub struct {
	searches []domain.SavedSearch
	err      error
}

func (s browserContextSavedSearchStub) SavedSearches(context.Context, int64) ([]domain.SavedSearch, error) {
	return s.searches, s.err
}

type browserContextNotificationStub struct {
	notifications []domain.Notification
	unread        int
	err           error
}

func (s browserContextNotificationStub) Notifications(context.Context, int64, int) ([]domain.Notification, int, error) {
	return s.notifications, s.unread, s.err
}

type browserContextAccessStub struct {
	filter func([]domain.Page) []domain.Page
	err    error
}

func (browserContextAccessStub) CanView(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (browserContextAccessStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (s browserContextAccessStub) FilterPages(_ context.Context, _ domain.User, pages []domain.Page) ([]domain.Page, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.filter == nil {
		return pages, nil
	}
	return s.filter(pages), nil
}

func TestAddPluginFeaturesAddsGenericSyntaxSettings(t *testing.T) {
	features := map[string]bool{}
	addPluginFeatures(features, plugin.LoadedPlugin{
		Enabled: true,
		Manifest: pluginpackage.Manifest{
			ID: "io.example.tables",
			Modules: []pluginpackage.Module{
				{Type: "markdown-syntax", ID: "grammar", Syntax: "tables"},
				{Type: "settings", ID: "sorting", Requires: []string{"grammar"}},
				{Type: "settings", ID: "filtering", Requires: []string{"grammar"}},
			},
		},
		Settings: map[string]bool{"sorting": true, "filtering": false},
	})

	assert.True(t, features["io.example.tables"])
	assert.True(t, features["io.example.tables.sorting"])
	assert.False(t, features["io.example.tables.filtering"])
	assert.True(t, features["markdown-syntax.tables"])
	assert.True(t, features["markdown-syntax.tables.sorting"])
	assert.False(t, features["markdown-syntax.tables.filtering"])
}

func TestAddPluginFeaturesDoesNotExposeDisabledSyntax(t *testing.T) {
	features := map[string]bool{}
	addPluginFeatures(features, plugin.LoadedPlugin{
		Enabled: false,
		Manifest: pluginpackage.Manifest{
			ID:      "io.example.tables",
			Modules: []pluginpackage.Module{{Type: "markdown-syntax", ID: "grammar", Syntax: "tables"}},
		},
	})

	assert.False(t, features["io.example.tables"])
	assert.NotContains(t, features, "markdown-syntax.tables")
}

// newTestBrowserContext wires independent fakes into the shared application query.
func newTestBrowserContext(preferences interface {
	Preferences(context.Context, int64) (domain.UserPreferences, error)
},
	navigation interface {
		NavigationPages(context.Context) ([]domain.Page, error)
		NavigationIcons(context.Context) (map[string]string, error)
	},
	catalog interface {
		Favorites(context.Context, int64) ([]domain.Page, error)
		RecentViewed(context.Context, int64, int) ([]domain.Page, error)
	},
	settings interface {
		ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	},
	searches interface {
		SavedSearches(context.Context, int64) ([]domain.SavedSearch, error)
	},
	notifications interface {
		Notifications(context.Context, int64, int) ([]domain.Notification, int, error)
	},
	access interface {
		FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error)
	},
	_ any) *BrowserContext {
	return NewBrowserContext(viewer.New(preferences, navigation, catalog, settings, searches, notifications, access), nil)
}
