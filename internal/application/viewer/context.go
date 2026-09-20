package viewer

import (
	"context"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/renderprofile"
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

// Context contains only data shared by authenticated browser screens.
type Context struct {
	User                domain.User
	Preferences         domain.UserPreferences
	Pages               []domain.Page
	NavigationIcons     map[string]string
	Settings            domain.ApplicationSettings
	SavedSearches       []domain.SavedSearch
	Notifications       []domain.Notification
	UnreadNotifications int
}

// Query loads the shared browser context through consumer-owned read ports.
type Query struct {
	preferences   preferenceReader
	navigation    navigationReader
	catalog       sidebarCatalogReader
	settings      settingsReader
	savedSearches savedSearchReader
	notifications notificationReader
	access        accessReader
}

// New constructs the shared browser query.
func New(preferences preferenceReader, navigation navigationReader, catalog sidebarCatalogReader,
	settings settingsReader, savedSearches savedSearchReader, notifications notificationReader, access accessReader) *Query {
	return &Query{preferences, navigation, catalog, settings, savedSearches, notifications, access}
}

// Load reads shared data for one actor; administration screens omit the page tree.
func (q *Query) Load(ctx context.Context, actor domain.User, includeNavigation bool) (Context, error) {
	result := Context{User: actor}
	var err error
	stop := renderprofile.FromContext(ctx).Measure("view_preferences")
	result.Preferences, err = q.preferences.Preferences(ctx, actor.ID)
	stop()
	if err != nil {
		return Context{}, err
	}
	if includeNavigation {
		result.Pages, err = q.navigation.NavigationPages(ctx)
		if err != nil {
			return Context{}, err
		}
		result.Pages, err = q.access.FilterPages(ctx, actor, result.Pages)
		if err != nil {
			return Context{}, err
		}
		result.NavigationIcons, err = q.navigation.NavigationIcons(ctx)
		if err != nil {
			return Context{}, err
		}
	}
	result.Settings, err = q.settings.ApplicationSettings(ctx)
	if err != nil {
		return Context{}, err
	}
	result.SavedSearches, err = q.savedSearches.SavedSearches(ctx, actor.ID)
	if err != nil {
		return Context{}, err
	}
	result.Notifications, result.UnreadNotifications, err = q.notifications.Notifications(ctx, actor.ID, 8)
	if err != nil {
		return Context{}, err
	}
	return result, nil
}

// PersonalLists binds lazy plugin capability queries to an authenticated actor.
func (q *Query) PersonalLists(actor domain.User) PersonalLists {
	return PersonalLists{catalog: q.catalog, access: q.access, user: actor}
}
