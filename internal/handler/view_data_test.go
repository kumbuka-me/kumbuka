package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicViewData(t *testing.T) {
	t.Parallel()

	availableThemes := []themes.Theme{
		{Title: "Light", ColorScheme: "light"},
		{Title: "Dark", ColorScheme: "dark"},
	}
	views := &Views{
		version:      "v1.2.3",
		commit:       "abc123",
		assetVersion: "0123456789abcdef",
		themes:       availableThemes,
		runtime:      RuntimeInfo{PublicURL: "https://kumbuka.example.test"},
	}

	data, err := publicViewData(views, "Sign in")

	require.NoError(t, err)
	assert.Equal(t, "Sign in", data.Title)
	assert.Equal(t, themes.DefaultTheme, data.ActiveTheme)
	assert.Equal(t, themes.DefaultTheme, data.Preferences.Theme)
	assert.Equal(t, "v1.2.3", data.Version)
	assert.Equal(t, "abc123", data.Commit)
	assert.Equal(t, "0123456789abcdef", data.AssetVersion)
	assert.Equal(t, views.runtime, data.Runtime)
	assert.Equal(t, availableThemes, data.Themes)
	assert.Contains(t, string(data.ThemeData), `"title":"Light"`)
	assert.Contains(t, string(data.ThemeData), `"title":"Dark"`)
}

func TestViewDataLoaderLoad(t *testing.T) {
	t.Parallel()

	t.Run("loads authenticated navigation and shared chrome data", func(t *testing.T) {
		t.Parallel()

		user := domain.User{ID: 42, Username: "editor", Role: "editor", Enabled: true}
		preferences := domain.DefaultUserPreferences()
		preferences.Theme = "Dark"
		preferences.TypographySize = ""
		preferences.ShowNavigationPageCounts = true
		preferences.ExpandedNavigation = []string{"platforms"}

		loader := NewViewDataLoader(
			viewDataPreferenceStub{preferences: preferences},
			viewDataNavigationStub{
				pages: []domain.Page{
					{ID: 1, Slug: "platforms", Title: "Platforms"},
					{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"},
					{ID: 99, Slug: "restricted", Title: "Restricted"},
				},
				icons: map[string]string{"platforms": "folder-lucide"},
			},
			viewDataCatalogStub{
				favorites: []domain.Page{{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"}},
				recent: []domain.Page{
					{ID: 2, Slug: "platforms/kubernetes", Title: "Kubernetes"},
					{ID: 4, Slug: "platforms/nomad", Title: "Nomad"},
					{ID: 5, Slug: "platforms/consul", Title: "Consul"},
				},
			},
			viewDataSettingsStub{settings: domain.ApplicationSettings{
				ContentLanguage: "de-CH",
				Rendering: domain.RenderingSettings{
					DefaultTypographySize: domain.TypographySizeLarge,
				},
			}},
			viewDataSavedSearchStub{searches: []domain.SavedSearch{{ID: 7, Name: "Production", Query: "tag:prod"}}},
			viewDataNotificationStub{
				notifications: []domain.Notification{{ID: 8, Title: "Mention"}},
				unread:        1,
			},
			viewDataAccessStub{filter: func(pages []domain.Page) []domain.Page {
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
		views := &Views{
			version:      "v1.2.3",
			commit:       "abc123",
			assetVersion: "0123456789abcdef",
			themes: []themes.Theme{
				{Title: "Light", ColorScheme: "light"},
				{Title: "Dark", ColorScheme: "dark"},
			},
			runtime: RuntimeInfo{PublicURL: "https://kumbuka.example.test"},
		}
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/pages/platforms/kubernetes", nil),
			user,
		)

		data, err := loader.Load(request, views, "Kubernetes")

		require.NoError(t, err)
		assert.Equal(t, "Kubernetes", data.Title)
		assert.Equal(t, user, data.User)
		assert.True(t, data.CanEdit)
		assert.Equal(t, domain.TypographySizeLarge, data.TypographySize)
		assert.Equal(t, "Dark", data.ActiveTheme)
		assert.Equal(t, "Dark", data.Preferences.Theme)
		assert.Equal(t, "platforms/kubernetes", data.NewPageParent)
		assert.Equal(t, domain.PageStatuses(), data.PageStatuses)
		assert.Equal(t, "de-CH", data.PageContentLanguage)
		assert.Equal(t, "v1.2.3", data.Version)
		assert.Equal(t, "abc123", data.Commit)
		assert.Equal(t, "0123456789abcdef", data.AssetVersion)
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
		loader := NewViewDataLoader(
			viewDataPreferenceStub{preferences: preferences},
			nil,
			nil,
			viewDataSettingsStub{settings: domain.ApplicationSettings{}},
			viewDataSavedSearchStub{},
			viewDataNotificationStub{},
			nil,
			nil,
		)
		views := &Views{themes: []themes.Theme{{Title: "Dark", ColorScheme: "dark"}}}
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/admin", nil), user)

		data, err := loader.Load(request, views, "Administration")

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
		loader := NewViewDataLoader(
			viewDataPreferenceStub{err: wantErr},
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)

		_, err := loader.Load(
			httptest.NewRequest(http.MethodGet, "/admin", nil),
			&Views{},
			"Administration",
		)

		assert.ErrorIs(t, err, wantErr)
	})

	t.Run("returns settings errors", func(t *testing.T) {
		t.Parallel()

		wantErr := errors.New("load settings")
		loader := NewViewDataLoader(
			viewDataPreferenceStub{preferences: domain.DefaultUserPreferences()},
			nil,
			nil,
			viewDataSettingsStub{err: wantErr},
			nil,
			nil,
			nil,
			nil,
		)

		_, err := loader.Load(
			httptest.NewRequest(http.MethodGet, "/admin", nil),
			&Views{},
			"Administration",
		)

		assert.ErrorIs(t, err, wantErr)
	})
}

func TestViewData(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodGet, "/pages/platforms", nil)
	views := &Views{}
	loader := viewDataServiceStub{load: func(gotRequest *http.Request, gotViews *Views, title string) (ViewData, error) {
		assert.Same(t, request, gotRequest)
		assert.Same(t, views, gotViews)
		assert.Equal(t, "Platforms", title)
		return ViewData{Title: title}, nil
	}}

	data, err := viewData(request, loader, views, "Platforms")

	require.NoError(t, err)
	assert.Equal(t, "Platforms", data.Title)
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

type viewDataPreferenceStub struct {
	preferences domain.UserPreferences
	err         error
}

func (s viewDataPreferenceStub) Preferences(context.Context, int64) (domain.UserPreferences, error) {
	return s.preferences, s.err
}

func (viewDataPreferenceStub) SavePreferences(context.Context, int64, domain.UserPreferences) error {
	return nil
}

func (viewDataPreferenceStub) SetShowPageContents(context.Context, int64, bool) error {
	return nil
}

func (viewDataPreferenceStub) SetExpandedNavigation(context.Context, int64, []string) error {
	return nil
}

func (viewDataPreferenceStub) SetSidebarWidth(context.Context, int64, int) error {
	return nil
}

type viewDataNavigationStub struct {
	pages []domain.Page
	icons map[string]string
	err   error
}

func (s viewDataNavigationStub) NavigationPages(context.Context) ([]domain.Page, error) {
	return s.pages, s.err
}

func (viewDataNavigationStub) NavigationItems(context.Context) ([]domain.NavigationItem, error) {
	return nil, nil
}

func (s viewDataNavigationStub) NavigationIcons(context.Context) (map[string]string, error) {
	return s.icons, s.err
}

func (viewDataNavigationStub) SetNavigationIcon(context.Context, string, string) error {
	return nil
}

type viewDataCatalogStub struct {
	favorites []domain.Page
	recent    []domain.Page
}

func (s viewDataCatalogStub) Favorites(context.Context, int64) ([]domain.Page, error) {
	return s.favorites, nil
}

func (s viewDataCatalogStub) RecentViewed(context.Context, int64, int) ([]domain.Page, error) {
	return s.recent, nil
}

type viewDataSettingsStub struct {
	settings domain.ApplicationSettings
	err      error
}

func (s viewDataSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, s.err
}

func (viewDataSettingsStub) PDFHeaders(context.Context) ([]domain.PDFHeader, error) {
	return nil, nil
}

func (viewDataSettingsStub) PDFRequestHeaders(context.Context) ([]domain.PDFHeader, error) {
	return nil, nil
}

func (viewDataSettingsStub) ResolvePDFRequestHeaders(context.Context, []service.PDFHeaderInput) ([]domain.PDFHeader, error) {
	return nil, nil
}

func (viewDataSettingsStub) RevealPDFHeader(context.Context, int64) (string, error) {
	return "", nil
}

func (viewDataSettingsStub) SaveApplicationSettings(context.Context, domain.ApplicationSettings, int64) error {
	return nil
}

func (viewDataSettingsStub) SavePDFSettings(context.Context, string, []service.PDFHeaderInput, int64) error {
	return nil
}

func (viewDataSettingsStub) SaveAuthenticationSettings(context.Context, domain.AuthenticationSettings, int64) error {
	return nil
}

func (viewDataSettingsStub) RecordLocalPasswordUpdated(context.Context, domain.User) {}

type viewDataSavedSearchStub struct {
	searches []domain.SavedSearch
	err      error
}

func (s viewDataSavedSearchStub) SavedSearches(context.Context, int64) ([]domain.SavedSearch, error) {
	return s.searches, s.err
}

type viewDataNotificationStub struct {
	notifications []domain.Notification
	unread        int
	err           error
}

func (s viewDataNotificationStub) Notifications(context.Context, int64, int) ([]domain.Notification, int, error) {
	return s.notifications, s.unread, s.err
}

type viewDataAccessStub struct {
	filter func([]domain.Page) []domain.Page
	err    error
}

func (viewDataAccessStub) CanView(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (viewDataAccessStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (s viewDataAccessStub) FilterPages(_ context.Context, _ domain.User, pages []domain.Page) ([]domain.Page, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.filter == nil {
		return pages, nil
	}
	return s.filter(pages), nil
}

type viewDataServiceStub struct {
	load func(*http.Request, *Views, string) (ViewData, error)
}

func (s viewDataServiceStub) Load(r *http.Request, views *Views, title string) (ViewData, error) {
	return s.load(r, views, title)
}
