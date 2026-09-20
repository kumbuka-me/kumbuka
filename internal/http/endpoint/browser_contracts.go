package endpoint

import (
	"context"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// viewDataService loads the browser layout shared by HTML endpoints.
type viewDataService interface {
	Load(*http.Request, *webview.Views, string) (webview.Layout, error)
}

// preferenceService owns the current user's presentation preferences.
type preferenceService interface {
	Preferences(context.Context, int64) (domain.UserPreferences, error)
	SavePreferences(context.Context, int64, domain.UserPreferences) error
	SetShowPageContents(context.Context, int64, bool) error
	SetExpandedNavigation(context.Context, int64, []string) error
	SetSidebarWidth(context.Context, int64, int) error
}

// savedSearchService mutates searches saved by the current user.
type savedSearchService interface {
	SaveSavedSearch(context.Context, int64, int64, string, string, bool) error
	DeleteSavedSearch(context.Context, int64, int64) error
}

// notificationService owns the current user's notification inbox.
type notificationService interface {
	Notifications(context.Context, int64, int) (notifications []domain.Notification, unread int, err error)
	MarkNotificationRead(context.Context, int64, int64) error
	MarkAllNotificationsRead(context.Context, int64) error
	OpenNotification(context.Context, int64, int64) (string, error)
}
