package endpoint

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Unused capabilities remain embedded; an unexpected call fails the test.
type emptyContractServices struct {
	navigationService
	groupReader
	userDirectoryService
	imageService
	attachmentService
}

func (emptyContractServices) ListPages(context.Context, int) ([]domain.Page, error) { return nil, nil }

func (emptyContractServices) Search(context.Context, string, int) ([]domain.Page, error) {
	return nil, nil
}
func (emptyContractServices) Tags(context.Context) ([]string, error) { return nil, nil }
func (emptyContractServices) AssignableGroups(context.Context, domain.User) ([]domain.Group, error) {
	return nil, nil
}

func (emptyContractServices) GroupMembers(context.Context, int64) ([]domain.User, error) {
	return nil, nil
}

func (emptyContractServices) SearchUsers(context.Context, string, int) ([]domain.User, error) {
	return nil, nil
}

func (emptyContractServices) NavigationPages(context.Context) ([]domain.Page, error) { return nil, nil }

func (emptyContractServices) PageAliases(context.Context) (map[string]string, error) { return nil, nil }

func (emptyContractServices) KnowledgeGraph(context.Context, int) (domain.KnowledgeGraph, error) {
	return domain.KnowledgeGraph{}, nil
}

func (emptyContractServices) Notifications(context.Context, int64, int) ([]domain.Notification, int, error) {
	return nil, 0, nil
}

func (emptyContractServices) MarkNotificationRead(context.Context, int64, int64) error { return nil }

func (emptyContractServices) MarkAllNotificationsRead(context.Context, int64) error { return nil }

func (emptyContractServices) OpenNotification(context.Context, int64, int64) (string, error) {
	return "", nil
}
func (emptyContractServices) Images(context.Context) ([]domain.Image, error) { return nil, nil }
func (emptyContractServices) SearchImages(context.Context, string, int, int) ([]domain.Image, error) {
	return nil, nil
}
func (emptyContractServices) SearchImagesByUser(context.Context, int64, string, int, int) ([]domain.Image, error) {
	return nil, nil
}
func (emptyContractServices) Attachments(context.Context) ([]domain.Attachment, error) {
	return nil, nil
}

func (emptyContractServices) CanView(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (emptyContractServices) CanEdit(context.Context, domain.User, string) (bool, error) {
	return true, nil
}

func (emptyContractServices) FilterPages(_ context.Context, _ domain.User, pages []domain.Page) ([]domain.Page, error) {
	return pages, nil
}

// Exercise real HTTP serialization, not just zero-value domain marshaling.
func TestEmptyAPICollectionContracts(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../test/contracts/http.json")
	require.NoError(t, err)
	var fixtures map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fixtures))
	services := emptyContractServices{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Run("pages", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		ListPages(services, services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("search", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		SearchAPI(services, services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("recent", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		Recent(services, services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("tags", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		Tags(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("groups", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		GroupsAPI(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("members", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		AdminGroupMembers(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("users", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		SearchAdminUsers(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("images", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		ListImages(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("attachments", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		ListAttachments(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_collection"]), response.Body.String())
	})

	t.Run("graph", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		KnowledgeGraphAPI(services, services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_graph"]), response.Body.String())
	})

	t.Run("catalog", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		EditorCatalog(services, services, nil, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_catalog"]), response.Body.String())
	})

	t.Run("notifications", func(t *testing.T) {
		t.Parallel()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/?q=nobody", nil), domain.User{ID: 1, Role: "admin", Enabled: true})
		request.SetPathValue("id", "1")
		response := httptest.NewRecorder()
		NotificationsAPI(services, logger)(response, request)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(fixtures["empty_notifications"]), response.Body.String())
	})
}
