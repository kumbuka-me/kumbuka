package endpoint

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeImageFilename(t *testing.T) {
	t.Parallel()

	t.Run("keeps safe png", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "architecture.png", appmedia.SanitizeImageFilename("architecture.png", "image/png"))
	})
	t.Run("normalizes spaces", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "my-diagram.png", appmedia.SanitizeImageFilename("my diagram.png", "image/png"))
	})
	t.Run("corrects extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "photo.jpg", appmedia.SanitizeImageFilename("photo.png", "image/jpeg"))
	})
	t.Run("allows jpeg extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "photo.jpeg", appmedia.SanitizeImageFilename("photo.jpeg", "image/jpeg"))
	})
	t.Run("adds extension", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "diagram.webp", appmedia.SanitizeImageFilename("diagram", "image/webp"))
	})
}

// imageListHandlerStub provides controllable image list handler behavior for tests.
type imageListHandlerStub struct {
	imageService
	// images configures or records the images value used by the fixture.
	images []domain.Image
	// searchResult configures the search result returned by the test double.
	searchResult []domain.Image
	// query records the query observed by the test double.
	query string
	// limit records the limit observed by the test double.
	limit int
	// offset records the offset observed by the test double.
	offset int
	// userID records the user ID observed by the test double.
	userID int64
	// usedSearch controls or records whether used search is active in the test.
	usedSearch bool
	// usedMine controls or records whether used mine is active in the test.
	usedMine bool
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
