package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeImageFilename(t *testing.T) {
	t.Parallel()

	t.Run("keeps safe png", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "architecture.png", service.SanitizeImageFilename("architecture.png", "image/png"))
	})
	t.Run("normalizes spaces", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "my-diagram.png", service.SanitizeImageFilename("my diagram.png", "image/png"))
	})
	t.Run("corrects extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "photo.jpg", service.SanitizeImageFilename("photo.png", "image/jpeg"))
	})
	t.Run("allows jpeg extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "photo.jpeg", service.SanitizeImageFilename("photo.jpeg", "image/jpeg"))
	})
	t.Run("adds extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "diagram.webp", service.SanitizeImageFilename("diagram", "image/webp"))
	})
}

func TestSupportedImageType(t *testing.T) {
	t.Parallel()

	t.Run("supports JPEG", func(t *testing.T) {
		t.Parallel()
		assert.True(t, service.SupportedImageType("image/jpeg"))
	})
	t.Run("supports PNG", func(t *testing.T) {
		t.Parallel()
		assert.True(t, service.SupportedImageType("image/png"))
	})
	t.Run("supports GIF", func(t *testing.T) {
		t.Parallel()
		assert.True(t, service.SupportedImageType("image/gif"))
	})
	t.Run("supports WebP", func(t *testing.T) {
		t.Parallel()
		assert.True(t, service.SupportedImageType("image/webp"))
	})
	t.Run("rejects SVG", func(t *testing.T) {
		t.Parallel()
		assert.False(t, service.SupportedImageType("image/svg+xml"))
	})
}

type imageListHandlerStub struct {
	imageService
	images       []domain.Image
	searchResult []domain.Image
	query        string
	limit        int
	offset       int
	userID       int64
	usedSearch   bool
	usedMine     bool
}

// Images returns all uploaded images with usage metadata.
func (s *imageListHandlerStub) Images(context.Context) ([]domain.Image, error) {
	return s.images, nil
}

// SearchImages returns a bounded set of images matching filename or uploader.
func (s *imageListHandlerStub) SearchImages(_ context.Context, query string, limit, offset int) ([]domain.Image, error) {
	s.usedSearch = true
	s.query = query
	s.limit = limit
	s.offset = offset
	return s.searchResult, nil
}

// SearchImagesByUser returns a bounded set of one user's images matching filename.
func (s *imageListHandlerStub) SearchImagesByUser(
	_ context.Context,
	userID int64,
	query string,
	limit, offset int,
) ([]domain.Image, error) {
	s.usedMine = true
	s.userID = userID
	s.query = query
	s.limit = limit
	s.offset = offset
	return s.searchResult, nil
}

func TestListImagesSupportsManagedPagination(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("plain request keeps complete image list behavior", func(t *testing.T) {
		t.Parallel()

		stub := &imageListHandlerStub{images: []domain.Image{{ID: 1, Filename: "one.png"}}}
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/api/images", nil),
			domain.User{ID: 7, Role: "editor", Enabled: true},
		)
		response := httptest.NewRecorder()

		ListImages(stub, logger)(response, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.False(t, stub.usedSearch)
		assert.False(t, stub.usedMine)

		var items []webview.MediaItem
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &items))
		require.Len(t, items, 1)
		assert.Equal(t, int64(1), items[0].ID)
	})

	t.Run("search passes query limit and offset", func(t *testing.T) {
		t.Parallel()

		stub := &imageListHandlerStub{}
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/api/images?q=diagram&limit=31&offset=30", nil),
			domain.User{ID: 7, Role: "admin", Enabled: true},
		)
		response := httptest.NewRecorder()

		ListImages(stub, logger)(response, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, stub.usedSearch)
		assert.False(t, stub.usedMine)
		assert.Equal(t, "diagram", stub.query)
		assert.Equal(t, 31, stub.limit)
		assert.Equal(t, 30, stub.offset)
	})

	t.Run("mine scope restricts search to current user", func(t *testing.T) {
		t.Parallel()

		stub := &imageListHandlerStub{}
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/api/images?scope=mine&q=logo&limit=31", nil),
			domain.User{ID: 42, Role: "editor", Enabled: true},
		)
		response := httptest.NewRecorder()

		ListImages(stub, logger)(response, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.True(t, stub.usedMine)
		assert.False(t, stub.usedSearch)
		assert.Equal(t, int64(42), stub.userID)
		assert.Equal(t, "logo", stub.query)
		assert.Equal(t, 31, stub.limit)
		assert.Zero(t, stub.offset)
	})

	t.Run("rejects unknown scope", func(t *testing.T) {
		t.Parallel()

		stub := &imageListHandlerStub{}
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/api/images?scope=other", nil),
			domain.User{ID: 7, Role: "editor", Enabled: true},
		)
		response := httptest.NewRecorder()

		ListImages(stub, logger)(response, request)

		assert.Equal(t, http.StatusBadRequest, response.Code)
		assert.False(t, stub.usedSearch)
		assert.False(t, stub.usedMine)
	})
}

func TestImageListRange(t *testing.T) {
	t.Parallel()

	t.Run("uses managed default", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		limit, offset, ok := imageListRange(response, "", "")

		require.True(t, ok)
		assert.Equal(t, managedImagePageSize, limit)
		assert.Zero(t, offset)
	})

	t.Run("accepts explicit range", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		limit, offset, ok := imageListRange(response, "31", "60")

		require.True(t, ok)
		assert.Equal(t, 31, limit)
		assert.Equal(t, 60, offset)
	})

	t.Run("rejects excessive limit", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		_, _, ok := imageListRange(response, "101", "0")

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})

	t.Run("rejects negative offset", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		_, _, ok := imageListRange(response, "30", "-1")

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, response.Code)
	})
}

func TestManagedImageItems(t *testing.T) {
	t.Parallel()

	t.Run("keeps one page", func(t *testing.T) {
		t.Parallel()

		images := make([]domain.Image, managedImagePageSize)
		items, hasMore := managedImageItems(images)

		assert.Len(t, items, managedImagePageSize)
		assert.False(t, hasMore)
	})

	t.Run("uses extra row only as more marker", func(t *testing.T) {
		t.Parallel()

		images := make([]domain.Image, managedImagePageSize+1)
		items, hasMore := managedImageItems(images)

		assert.Len(t, items, managedImagePageSize)
		assert.True(t, hasMore)
	})
}
