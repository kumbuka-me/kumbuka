package handler

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type notificationServiceStub struct {
	list    func(context.Context, int64, int) ([]domain.Notification, int, error)
	open    func(context.Context, int64, int64) (string, error)
	mark    func(context.Context, int64, int64) error
	markAll func(context.Context, int64) error
}

// Notifications delegates inbox reads to the configured test function.
func (s notificationServiceStub) Notifications(ctx context.Context, userID int64, limit int) ([]domain.Notification, int, error) {
	return s.list(ctx, userID, limit)
}

// OpenNotification delegates notification opening to the configured test function.
func (s notificationServiceStub) OpenNotification(ctx context.Context, userID, id int64) (string, error) {
	return s.open(ctx, userID, id)
}

// MarkNotificationRead delegates single-item reads to the configured test function.
func (s notificationServiceStub) MarkNotificationRead(ctx context.Context, userID, id int64) error {
	if s.mark == nil {
		return nil
	}

	return s.mark(ctx, userID, id)
}

// MarkAllNotificationsRead delegates whole-inbox reads to the configured test function.
func (s notificationServiceStub) MarkAllNotificationsRead(ctx context.Context, userID int64) error {
	if s.markAll == nil {
		return nil
	}

	return s.markAll(ctx, userID)
}

// notificationTestLogger returns a logger that discards handler diagnostics.
func notificationTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// notificationRequest creates one authenticated notification request.
func notificationRequest(method, target, id string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.SetPathValue("id", id)

	return auth.WithUser(request, domain.User{ID: 42, Enabled: true})
}

func TestNotificationsAPI(t *testing.T) {
	t.Parallel()

	t.Run("returns inbox and unread count without caching", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{list: func(_ context.Context, userID int64, limit int) ([]domain.Notification, int, error) {
			assert.Equal(t, int64(42), userID)
			assert.Equal(t, 30, limit)
			return []domain.Notification{{ID: 7, Title: "Mention"}}, 1, nil
		}}
		response := httptest.NewRecorder()

		NotificationsAPI(useCases, notificationTestLogger())(response, notificationRequest(http.MethodGet, "/api/notifications", ""))

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
		assert.JSONEq(t, `{"items":[{"id":7,"kind":"","title":"Mention","body":"","url":"","created_at":"0001-01-01T00:00:00Z"}],"unread":1}`, response.Body.String())
	})

	t.Run("returns internal error for persistence failure", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{list: func(context.Context, int64, int) ([]domain.Notification, int, error) {
			return nil, 0, errors.New("database unavailable")
		}}
		response := httptest.NewRecorder()

		NotificationsAPI(useCases, notificationTestLogger())(response, notificationRequest(http.MethodGet, "/api/notifications", ""))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("requires authentication", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{list: func(context.Context, int64, int) ([]domain.Notification, int, error) {
			t.Fatal("service must not be called")
			return nil, 0, nil
		}}
		response := httptest.NewRecorder()

		NotificationsAPI(useCases, notificationTestLogger())(response, httptest.NewRequest(http.MethodGet, "/api/notifications", nil))

		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})
}

func TestOpenNotification(t *testing.T) {
	t.Parallel()

	t.Run("redirects to stored local destination", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(_ context.Context, userID, id int64) (string, error) {
			assert.Equal(t, int64(42), userID)
			assert.Equal(t, int64(7), id)
			return "/pages/example#comments", nil
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, "/pages/example#comments", response.Header().Get("Location"))
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	})

	t.Run("returns not found for missing or foreign item", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			return "", domain.ErrNotFound
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("returns internal error for persistence failure", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			return "", errors.New("database unavailable")
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, http.StatusInternalServerError, response.Code)
	})

	t.Run("rejects invalid id before service call", func(t *testing.T) {
		t.Parallel()

		called := false
		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			called = true
			return "", nil
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/all/open", "all"))

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.False(t, called)
	})

	t.Run("requires authentication", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			t.Fatal("service must not be called")
			return "", nil
		}}
		request := httptest.NewRequest(http.MethodPost, "/notifications/7/open", nil)
		request.SetPathValue("id", "7")
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, request)

		assert.Equal(t, http.StatusUnauthorized, response.Code)
	})

	t.Run("falls back to home for external destination", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			return "https://example.com", nil
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, "/", response.Header().Get("Location"))
	})

	t.Run("falls back to home for protocol relative destination", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			return "//example.com", nil
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, "/", response.Header().Get("Location"))
	})

	t.Run("falls back to home for backslash destination", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) {
			return "/\\example.com", nil
		}}
		response := httptest.NewRecorder()

		OpenNotification(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/notifications/7/open", "7"))

		assert.Equal(t, "/", response.Header().Get("Location"))
	})
}

func TestMarkNotificationRead(t *testing.T) {
	t.Parallel()

	t.Run("marks one notification", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{
			open: func(context.Context, int64, int64) (string, error) { return "", nil },
			mark: func(_ context.Context, userID, id int64) error {
				assert.Equal(t, int64(42), userID)
				assert.Equal(t, int64(7), id)
				return nil
			},
			markAll: func(context.Context, int64) error {
				t.Fatal("mark all must not be called")
				return nil
			},
		}
		response := httptest.NewRecorder()

		MarkNotificationRead(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/api/notifications/7/read", "7"))

		assert.Equal(t, http.StatusNoContent, response.Code)
		assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	})

	t.Run("marks the complete inbox", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{
			open: func(context.Context, int64, int64) (string, error) { return "", nil },
			mark: func(context.Context, int64, int64) error {
				t.Fatal("single mark must not be called")
				return nil
			},
			markAll: func(_ context.Context, userID int64) error {
				assert.Equal(t, int64(42), userID)
				return nil
			},
		}
		response := httptest.NewRecorder()

		MarkNotificationRead(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/api/notifications/all/read", "all"))

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("rejects invalid id", func(t *testing.T) {
		t.Parallel()

		useCases := notificationServiceStub{open: func(context.Context, int64, int64) (string, error) { return "", nil }}
		response := httptest.NewRecorder()

		MarkNotificationRead(useCases, notificationTestLogger())(response, notificationRequest(http.MethodPost, "/api/notifications/nope/read", "nope"))

		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}

func TestNotificationID(t *testing.T) {
	t.Parallel()

	t.Run("accepts positive id", func(t *testing.T) {
		t.Parallel()

		id, err := notificationID("7")

		require.NoError(t, err)
		assert.Equal(t, int64(7), id)
	})

	t.Run("rejects zero", func(t *testing.T) {
		t.Parallel()

		_, err := notificationID("0")

		assert.Error(t, err)
	})

	t.Run("rejects nonnumeric id", func(t *testing.T) {
		t.Parallel()

		_, err := notificationID("all")

		assert.Error(t, err)
	})
}
