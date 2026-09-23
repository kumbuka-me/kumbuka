package viewer

import (
	"context"
	"errors"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

// viewDataPreferenceStub provides controllable view data preference behavior for tests.
type viewDataPreferenceStub struct {
	// preferences configures or records the preferences value used by the fixture.
	preferences domain.UserPreferences
	// err configures the error returned by the test double.
	err error
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

// viewDataNavigationStub provides controllable view data navigation behavior for tests.
type viewDataNavigationStub struct {
	// pages records the pages observed by the test double.
	pages []domain.Page
	// icons configures the icons used by the fixture.
	icons map[string]string
	// err configures the error returned by the test double.
	err error
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

// viewDataCatalogStub provides controllable view data catalog behavior for tests.
type viewDataCatalogStub struct {
	// favorites configures or records the favorites value used by the fixture.
	favorites []domain.Page
	// recent configures or records the recent value used by the fixture.
	recent []domain.Page
}

func (s viewDataCatalogStub) Favorites(context.Context, int64) ([]domain.Page, error) {
	return s.favorites, nil
}

func (s viewDataCatalogStub) RecentViewed(context.Context, int64, int) ([]domain.Page, error) {
	return s.recent, nil
}

// viewDataSettingsStub provides controllable view data settings behavior for tests.
type viewDataSettingsStub struct {
	// settings records the tings passed to set operations.
	settings domain.ApplicationSettings
	// err configures the error returned by the test double.
	err error
}

func (s viewDataSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, s.err
}

// viewDataSavedSearchStub provides controllable view data saved search behavior for tests.
type viewDataSavedSearchStub struct {
	// searches configures or records the searches value used by the fixture.
	searches []domain.SavedSearch
	// err configures the error returned by the test double.
	err error
}

func (s viewDataSavedSearchStub) SavedSearches(context.Context, int64) ([]domain.SavedSearch, error) {
	return s.searches, s.err
}

// viewDataNotificationStub provides controllable view data notification behavior for tests.
type viewDataNotificationStub struct {
	// notifications configures or records the notifications value used by the fixture.
	notifications []domain.Notification
	// unread configures or records the unread value used by the fixture.
	unread int
	// err configures the error returned by the test double.
	err error
}

func (s viewDataNotificationStub) Notifications(context.Context, int64, int) ([]domain.Notification, int, error) {
	return s.notifications, s.unread, s.err
}

// viewDataAccessStub provides controllable view data access behavior for tests.
type viewDataAccessStub struct {
	// filter provides the callback invoked by the test double.
	filter func([]domain.Page) []domain.Page
	// err configures the error returned by the test double.
	err error
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
