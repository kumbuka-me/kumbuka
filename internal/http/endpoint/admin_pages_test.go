package endpoint

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
)

// pageBulkServiceStub provides controllable page bulk service behavior for tests.
type pageBulkServiceStub struct {
	// calls counts calls observed by the test double.
	calls int
	// input records the input observed by the test double.
	input apppages.BulkPageInput
}

func (s *pageBulkServiceStub) Bulk(_ context.Context, input apppages.BulkPageInput) error {
	s.calls++
	s.input = input
	return nil
}

func TestUniqueNonEmpty(t *testing.T) {
	t.Parallel()

	t.Run("trims deduplicates and preserves order", func(t *testing.T) {
		t.Parallel()

		values := []string{" guide ", "", "api", "guide", " api ", "reference"}

		result := uniqueNonEmpty(values)

		assert.Equal(t, []string{"guide", "api", "reference"}, result)
	})

	t.Run("does not mutate input", func(t *testing.T) {
		t.Parallel()

		values := []string{" guide ", "api", "guide"}
		original := slices.Clone(values)

		_ = uniqueNonEmpty(values)

		assert.Equal(t, original, values)
	})
}

func TestBulkAdminPagesRejectsMalformedGroupID(t *testing.T) {
	t.Parallel()

	service := &pageBulkServiceStub{}
	form := url.Values{
		"action":   {"group"},
		"slug":     {"docs/start"},
		"group_id": {"not-a-number"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/pages/bulk", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	BulkAdminPages(service, nil, nil, slog.Default())(response, request)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Zero(t, service.calls)
}

func TestBulkAdminPagesParsesGroupIDForGroupAction(t *testing.T) {
	t.Parallel()

	service := &pageBulkServiceStub{}
	form := url.Values{
		"action":   {"group"},
		"slug":     {"docs/start"},
		"group_id": {"42"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/pages/bulk", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	BulkAdminPages(service, nil, nil, slog.Default())(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, 1, service.calls)
	assert.Equal(t, int64(42), service.input.GroupID)
}
