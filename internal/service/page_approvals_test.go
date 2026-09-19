package service

import (
	"context"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageApprovalValidation(t *testing.T) {
	t.Parallel()

	t.Run("request requires page path", func(t *testing.T) {
		t.Parallel()

		_, err := NewPages(nil, slog.Default()).RequestReview(context.Background(), PageReviewRequestInput{
			Slug:  " ",
			Actor: domain.User{Role: "editor"},
		})
		validation, ok := err.(*domain.ValidationError)

		require.True(t, ok)
		assert.Equal(t, "slug", validation.Fields[0].Field)
	})

	t.Run("viewer cannot request review", func(t *testing.T) {
		t.Parallel()

		_, err := NewPages(nil, slog.Default()).RequestReview(context.Background(), PageReviewRequestInput{
			Slug:  "guide",
			Actor: domain.User{Role: "viewer"},
		})

		assert.ErrorIs(t, err, domain.ErrForbidden)
	})

	t.Run("decision validates value before persistence", func(t *testing.T) {
		t.Parallel()

		err := NewPages(nil, slog.Default()).DecideReview(context.Background(), PageReviewDecisionInput{
			ID:       1,
			Slug:     "guide",
			Decision: "maybe",
			Actor:    domain.User{Role: "admin"},
		})
		validation, ok := err.(*domain.ValidationError)

		require.True(t, ok)
		assert.Equal(t, "decision", validation.Fields[0].Field)
	})

	t.Run("administrator can review without target lookup", func(t *testing.T) {
		t.Parallel()

		allowed, err := NewPages(nil, slog.Default()).CanReview(context.Background(), "guide", domain.User{Role: "admin"})

		require.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("requester can manage pending review", func(t *testing.T) {
		t.Parallel()

		request := domain.PageReviewRequest{ID: 7, RequestedBy: 42, Status: domain.PageReviewStatusPending}
		allowed := NewPages(nil, slog.Default()).CanManageReview(request, domain.User{ID: 42, Role: "editor"})

		assert.True(t, allowed)
	})

	t.Run("completed review cannot be managed", func(t *testing.T) {
		t.Parallel()

		request := domain.PageReviewRequest{ID: 7, RequestedBy: 42, Status: domain.PageReviewStatusApproved}
		allowed := NewPages(nil, slog.Default()).CanManageReview(request, domain.User{ID: 42, Role: "editor"})

		assert.False(t, allowed)
	})
}

func TestNormalizeReviewerUsernames(t *testing.T) {
	t.Parallel()

	result := normalizeReviewerUsernames([]string{" @alice ", "bob", "@ALICE", "", " @carol"})

	assert.Equal(t, []string{"alice", "bob", "carol"}, result)
}

type reviewTargetRepositoryStub struct {
	pageRepository
	active domain.PageReviewRequest
	page   domain.Page
	users  []domain.User
	group  domain.Group
	groups []domain.Group
}

func (r reviewTargetRepositoryStub) PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error) {
	return r.active, nil
}

func (r reviewTargetRepositoryStub) ReviewUsers(context.Context, []string) ([]domain.User, error) {
	return r.users, nil
}

func (r reviewTargetRepositoryStub) ReviewGroup(context.Context, int64) (domain.Group, error) {
	if r.group.ID == 0 {
		return domain.Group{}, domain.ErrNotFound
	}
	return r.group, nil
}

func (r reviewTargetRepositoryStub) ReviewGroups(context.Context) ([]domain.Group, error) {
	return r.groups, nil
}

func (r reviewTargetRepositoryStub) GetPage(context.Context, string) (domain.Page, error) {
	return r.page, nil
}

func TestReviewGroupsUsesReviewTargetProjection(t *testing.T) {
	t.Parallel()

	want := []domain.Group{{ID: 2, Name: "Editors"}, {ID: 4, Name: "Security"}}
	got, err := NewPages(reviewTargetRepositoryStub{groups: want}, slog.Default()).ReviewGroups(context.Background())

	require.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestResolveReviewTargets(t *testing.T) {
	t.Parallel()

	t.Run("uses owner group when no explicit target is selected", func(t *testing.T) {
		t.Parallel()

		repository := reviewTargetRepositoryStub{page: domain.Page{OwnerGroupID: 9}}
		userIDs, groupID, err := NewPages(repository, slog.Default()).resolveReviewTargets(context.Background(), "guide", nil, 0)

		require.NoError(t, err)
		assert.Empty(t, userIDs)
		assert.Equal(t, int64(9), groupID)
	})

	t.Run("keeps an individual-only request without owner fallback", func(t *testing.T) {
		t.Parallel()

		repository := reviewTargetRepositoryStub{
			page:  domain.Page{OwnerGroupID: 9},
			users: []domain.User{{ID: 4, Username: "alice", Role: "editor"}},
		}
		userIDs, groupID, err := NewPages(repository, slog.Default()).resolveReviewTargets(context.Background(), "guide", []string{"@alice"}, 0)

		require.NoError(t, err)
		assert.Equal(t, []int64{4}, userIDs)
		assert.Zero(t, groupID)
	})

	t.Run("rejects an unresolved individual reviewer", func(t *testing.T) {
		t.Parallel()

		repository := reviewTargetRepositoryStub{}
		_, _, err := NewPages(repository, slog.Default()).resolveReviewTargets(context.Background(), "guide", []string{"@missing"}, 0)
		validation, ok := err.(*domain.ValidationError)

		require.True(t, ok)
		assert.Equal(t, "reviewers", validation.Fields[0].Field)
	})
}

func TestRequestReviewRequiresChangesToBeAddressed(t *testing.T) {
	t.Parallel()

	repository := reviewTargetRepositoryStub{
		active: domain.PageReviewRequest{ID: 3, Status: domain.PageReviewStatusChangesRequested},
	}
	_, err := NewPages(repository, slog.Default()).RequestReview(context.Background(), PageReviewRequestInput{
		Slug:  "guide",
		Actor: domain.User{ID: 7, Role: "editor"},
	})

	assert.ErrorIs(t, err, domain.ErrReviewChangesRequired)
}
