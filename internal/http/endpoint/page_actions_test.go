package endpoint

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageApprovalRouteStub captures review mutations received from route-bound handlers.
type pageApprovalRouteStub struct {
	pageApprovalService
	// update captures the last review update request.
	update apppages.PageReviewUpdateInput
	// cancelledSlug captures the page path used to cancel a review.
	cancelledSlug string
	// decision captures the last review decision request.
	decision apppages.PageReviewDecisionInput
}

// UpdateReview captures one review update request.
func (s *pageApprovalRouteStub) UpdateReview(_ context.Context, input apppages.PageReviewUpdateInput) (domain.PageReviewRequest, error) {
	s.update = input
	return domain.PageReviewRequest{}, nil
}

// CancelReview captures one review cancellation request.
func (s *pageApprovalRouteStub) CancelReview(_ context.Context, _ int64, slug string, _ domain.User) error {
	s.cancelledSlug = slug
	return nil
}

// DecideReview captures one review decision request.
func (s *pageApprovalRouteStub) DecideReview(_ context.Context, input apppages.PageReviewDecisionInput) error {
	s.decision = input
	return nil
}

// TestReviewerUsernames verifies reviewer mention input is normalized into usernames.
func TestReviewerUsernames(t *testing.T) {
	t.Parallel()

	t.Run("parses mention list", func(t *testing.T) {
		t.Parallel()

		result := reviewerUsernames("@alice @bob, @carol;dave")

		assert.Equal(t, []string{"alice", "bob", "carol", "dave"}, result)
	})

	t.Run("ignores empty separators", func(t *testing.T) {
		t.Parallel()

		result := reviewerUsernames("  , ; \n\t")

		assert.Empty(t, result)
	})
}

// TestPageReviewMutationHandlersUseRouteSlug verifies review mutations cannot substitute another page through form data.
func TestPageReviewMutationHandlersUseRouteSlug(t *testing.T) {
	t.Parallel()

	const routeSlug = "docs/start"

	t.Run("update", func(t *testing.T) {
		t.Parallel()

		useCases := &pageApprovalRouteStub{}
		request := httptest.NewRequest(http.MethodPost, "/pages/approval/update/7/docs/start", strings.NewReader("slug=other&note=check"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetPathValue("id", "7")
		request.SetPathValue("slug", routeSlug)
		response := httptest.NewRecorder()

		UpdatePageReview(useCases, slog.Default(), viewDataAccessStub{}).ServeHTTP(response, request)

		require.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, routeSlug, useCases.update.Slug)
	})

	t.Run("cancel", func(t *testing.T) {
		t.Parallel()

		useCases := &pageApprovalRouteStub{}
		request := httptest.NewRequest(http.MethodPost, "/pages/approval/cancel/7/docs/start", strings.NewReader("slug=other"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetPathValue("id", "7")
		request.SetPathValue("slug", routeSlug)
		response := httptest.NewRecorder()

		CancelPageReview(useCases, slog.Default(), viewDataAccessStub{}).ServeHTTP(response, request)

		require.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, routeSlug, useCases.cancelledSlug)
	})

	t.Run("decision", func(t *testing.T) {
		t.Parallel()

		useCases := &pageApprovalRouteStub{}
		request := httptest.NewRequest(http.MethodPost, "/pages/approval/decide/7/docs/start", strings.NewReader("slug=other&decision=approved"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetPathValue("id", "7")
		request.SetPathValue("slug", routeSlug)
		response := httptest.NewRecorder()

		DecidePageReview(useCases, slog.Default(), viewDataAccessStub{}).ServeHTTP(response, request)

		require.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, routeSlug, useCases.decision.Slug)
		assert.Equal(t, "approved", useCases.decision.Decision)
	})
}
