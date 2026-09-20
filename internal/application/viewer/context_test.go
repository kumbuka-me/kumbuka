package viewer

import (
	"context"
	"errors"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

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

func TestLoadUsesActorAndFiltersNavigation(t *testing.T) {
	t.Parallel()
	actor := domain.User{ID: 8}
	query := New(viewDataPreferenceStub{preferences: domain.DefaultUserPreferences()},
		viewDataNavigationStub{pages: []domain.Page{{Slug: "open"}, {Slug: "secret"}}}, nil,
		viewDataSettingsStub{}, viewDataSavedSearchStub{}, viewDataNotificationStub{unread: 3},
		viewDataAccessStub{filter: func(pages []domain.Page) []domain.Page { return pages[:1] }})
	result, err := query.Load(context.Background(), actor, true)
	require.NoError(t, err)
	assert.Equal(t, actor, result.User)
	assert.Equal(t, []domain.Page{{Slug: "open"}}, result.Pages)
	assert.Equal(t, 3, result.UnreadNotifications)
}

func TestLoadAdministrationOmitsNavigation(t *testing.T) {
	t.Parallel()
	query := New(viewDataPreferenceStub{}, nil, nil, viewDataSettingsStub{}, viewDataSavedSearchStub{}, viewDataNotificationStub{}, nil)
	result, err := query.Load(context.Background(), domain.User{}, false)
	require.NoError(t, err)
	assert.Nil(t, result.Pages)
}

func TestLoadPropagatesFailures(t *testing.T) {
	t.Parallel()
	failure := errors.New("repository unavailable")
	query := New(viewDataPreferenceStub{}, nil, nil, viewDataSettingsStub{err: failure}, nil, nil, nil)
	_, err := query.Load(context.Background(), domain.User{}, false)
	require.ErrorIs(t, err, failure)
}

func TestPersonalListsFilterBeforeLimiting(t *testing.T) {
	t.Parallel()
	query := New(nil, nil, viewDataCatalogStub{favorites: []domain.Page{{Slug: "secret"}, {Slug: "open"}, {Slug: "also-open"}}}, nil, nil, nil,
		viewDataAccessStub{filter: func(pages []domain.Page) []domain.Page { return pages[1:] }})
	result, err := query.PersonalLists(domain.User{ID: 8}).Favorites(context.Background(), 1)
	require.NoError(t, err)
	assert.Equal(t, []domain.Page{{Slug: "open"}}, result)
}
