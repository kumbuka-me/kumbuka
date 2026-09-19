package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewDiscussionRepositoryStub captures review feedback and suggestion application for service tests.
type reviewDiscussionRepositoryStub struct {
	// pageRepository supplies unused methods so tests only implement the exercised review boundary.
	pageRepository
	// request is the review request returned by lookup methods.
	request domain.PageReviewRequest
	// page is the page returned by review detail lookups and applied writes.
	page domain.Page
	// revision is the immutable reviewed revision returned by the repository.
	revision revision.Revision
	// comments contains persisted review feedback returned by the repository.
	comments []domain.PageReviewComment
	// added captures the latest review comment persisted through the service.
	added domain.PageReviewComment
	// appliedMarkdown captures the canonical Markdown persisted by suggestion application.
	appliedMarkdown string
	// appliedIDs captures the suggestion identifiers persisted atomically.
	appliedIDs []int64
	// appliedAll reports whether persistence was asked to verify the complete pending suggestion set.
	appliedAll bool
}

// PageReviewRequestByID returns the configured review request when it matches the expected page.
func (r *reviewDiscussionRepositoryStub) PageReviewRequestByID(_ context.Context, id int64, slug string) (domain.PageReviewRequest, error) {
	if r.request.ID != id || r.request.PageSlug != slug {
		return domain.PageReviewRequest{}, domain.ErrNotFound
	}

	return r.request, nil
}

// GetPage returns the configured page for review service tests.
func (r *reviewDiscussionRepositoryStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	if r.page.Slug != slug {
		return domain.Page{}, domain.ErrNotFound
	}

	return r.page, nil
}

// Revision returns the configured immutable review revision.
func (r *reviewDiscussionRepositoryStub) Revision(_ context.Context, slug string, number int) (revision.Revision, error) {
	if r.page.Slug != slug || r.revision.Number != number {
		return revision.Revision{}, domain.ErrNotFound
	}

	return r.revision, nil
}

// PageReviewComments returns the configured line feedback.
func (r *reviewDiscussionRepositoryStub) PageReviewComments(context.Context, int64) ([]domain.PageReviewComment, error) {
	return r.comments, nil
}

// CanReviewPage reports no explicit reviewer assignment unless the actor is an administrator.
func (*reviewDiscussionRepositoryStub) CanReviewPage(context.Context, string, int64) (bool, error) {
	return false, nil
}

// AddPageReviewComment captures one persisted line comment or suggestion.
func (r *reviewDiscussionRepositoryStub) AddPageReviewComment(
	_ context.Context,
	requestID int64,
	_ string,
	authorID int64,
	side string,
	startLine, endLine int,
	body string,
	suggestion bool,
	original, replacement string,
) (domain.PageReviewComment, error) {
	r.added = domain.PageReviewComment{
		ID:              41,
		ReviewRequestID: requestID,
		AuthorID:        authorID,
		Side:            side,
		StartLine:       startLine,
		EndLine:         endLine,
		Body:            body,
		IsSuggestion:    suggestion,
		Original:        original,
		Replacement:     replacement,
	}

	return r.added, nil
}

// ApplyPageReviewSuggestions captures the atomic suggestion write and returns the configured page.
func (r *reviewDiscussionRepositoryStub) ApplyPageReviewSuggestions(
	_ context.Context,
	_ int64,
	_ string,
	_ int,
	_ int64,
	suggestionIDs []int64,
	applyAll bool,
	markdown string,
	_ string,
	_ []string,
	_ *pluginusage.Index,
	_ domain.PageRender,
) (domain.Page, error) {
	r.appliedMarkdown = markdown
	r.appliedIDs = append([]int64(nil), suggestionIDs...)
	r.appliedAll = applyAll

	return r.page, nil
}

// LogAudit accepts best-effort audit writes in review service tests.
func (*reviewDiscussionRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

// NotifyPageWatchers accepts best-effort watcher notifications in review service tests.
func (*reviewDiscussionRepositoryStub) NotifyPageWatchers(context.Context, int64, string, string, string, string) error {
	return nil
}

// TestAddReviewCommentCapturesReviewedSource verifies suggestions persist the exact reviewed Markdown range rather than browser text.
func TestAddReviewCommentCapturesReviewedSource(t *testing.T) {
	t.Parallel()

	repository := &reviewDiscussionRepositoryStub{
		request: domain.PageReviewRequest{
			ID:             7,
			PageSlug:       "guide",
			RevisionNumber: 2,
			RequestedBy:    9,
			Status:         domain.PageReviewStatusPending,
		},
		page: domain.Page{Slug: "guide", Title: "Guide"},
		revision: revision.Revision{
			Number:           2,
			PreviousMarkdown: "first\nold value\nlast",
			Markdown:         "first\ncurrent value\nlast",
		},
	}
	pages := NewPages(repository, slog.Default())

	comment, err := pages.AddReviewComment(context.Background(), PageReviewCommentInput{
		ReviewID:    7,
		Slug:        "guide",
		Side:        domain.PageReviewCommentSideNew,
		StartLine:   2,
		EndLine:     2,
		Body:        "Use the production value.",
		Suggestion:  true,
		Replacement: "production value",
		Actor:       domain.User{ID: 1, Role: "admin"},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(41), comment.ID)
	assert.Equal(t, "current value", repository.added.Original)
	assert.Equal(t, "production value", repository.added.Replacement)
}

// TestApplyMarkdownSuggestions verifies applicable suggestions replace immutable line ranges without offset drift.
func TestApplyMarkdownSuggestions(t *testing.T) {
	t.Parallel()

	t.Run("applies multiple suggestions from bottom to top", func(t *testing.T) {
		t.Parallel()

		updated, err := applyMarkdownSuggestions("one\ntwo\nthree\nfour", []domain.PageReviewComment{
			{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "TWO"},
			{ID: 2, Side: domain.PageReviewCommentSideNew, StartLine: 4, EndLine: 4, Original: "four", Replacement: "FOUR\nFIVE"},
		})

		require.NoError(t, err)
		assert.Equal(t, "one\nTWO\nthree\nFOUR\nFIVE", updated)
	})

	t.Run("supports deletion", func(t *testing.T) {
		t.Parallel()

		updated, err := applyMarkdownSuggestions("one\ntwo\nthree", []domain.PageReviewComment{
			{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: ""},
		})

		require.NoError(t, err)
		assert.Equal(t, "one\nthree", updated)
	})

	t.Run("rejects overlapping ranges", func(t *testing.T) {
		t.Parallel()

		_, err := applyMarkdownSuggestions("one\ntwo\nthree", []domain.PageReviewComment{
			{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 1, EndLine: 2, Original: "one\ntwo", Replacement: "first"},
			{ID: 2, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 3, Original: "two\nthree", Replacement: "last"},
		})

		assert.ErrorIs(t, err, domain.ErrReviewSuggestionConflict)
	})

	t.Run("rejects stale original text", func(t *testing.T) {
		t.Parallel()

		_, err := applyMarkdownSuggestions("one\nchanged\nthree", []domain.PageReviewComment{
			{ID: 1, Side: domain.PageReviewCommentSideNew, StartLine: 2, EndLine: 2, Original: "two", Replacement: "TWO"},
		})

		assert.ErrorIs(t, err, domain.ErrStaleReview)
	})
}

// TestApplyAllReviewSuggestionsPersistsOneRevisionInput verifies all open suggestions are combined before persistence.
func TestApplyAllReviewSuggestionsPersistsOneRevisionInput(t *testing.T) {
	t.Parallel()

	repository := &reviewDiscussionRepositoryStub{
		request: domain.PageReviewRequest{
			ID:             7,
			PageSlug:       "guide",
			RevisionNumber: 3,
			RequestedBy:    9,
			Status:         domain.PageReviewStatusPending,
		},
		page: domain.Page{Slug: "guide", Title: "Guide"},
		revision: revision.Revision{
			Number:           3,
			PreviousMarkdown: "one\ntwo\nthree",
			Markdown:         "one\ntwo\nthree",
		},
		comments: []domain.PageReviewComment{
			{ID: 10, IsSuggestion: true, Side: domain.PageReviewCommentSideNew, StartLine: 1, EndLine: 1, Original: "one", Replacement: "ONE"},
			{ID: 11, IsSuggestion: true, Side: domain.PageReviewCommentSideNew, StartLine: 3, EndLine: 3, Original: "three", Replacement: "THREE"},
		},
	}
	pages := NewPages(repository, slog.Default())

	page, err := pages.ApplyAllReviewSuggestions(context.Background(), 7, "guide", domain.User{ID: 9, Role: "editor"})

	require.NoError(t, err)
	assert.Equal(t, "guide", page.Slug)
	assert.Equal(t, "ONE\ntwo\nTHREE", repository.appliedMarkdown)
	assert.Equal(t, []int64{10, 11}, repository.appliedIDs)
	assert.True(t, repository.appliedAll)
}

// TestApplyReviewSuggestionKeepsSingleSelectionSemantics verifies applying one suggestion does not require every pending suggestion.
func TestApplyReviewSuggestionKeepsSingleSelectionSemantics(t *testing.T) {
	t.Parallel()

	repository := &reviewDiscussionRepositoryStub{
		request: domain.PageReviewRequest{
			ID:             7,
			PageSlug:       "guide",
			RevisionNumber: 3,
			RequestedBy:    9,
			Status:         domain.PageReviewStatusPending,
		},
		page: domain.Page{Slug: "guide", Title: "Guide"},
		revision: revision.Revision{
			Number:           3,
			PreviousMarkdown: "one\ntwo\nthree",
			Markdown:         "one\ntwo\nthree",
		},
		comments: []domain.PageReviewComment{
			{ID: 10, IsSuggestion: true, Side: domain.PageReviewCommentSideNew, StartLine: 1, EndLine: 1, Original: "one", Replacement: "ONE"},
			{ID: 11, IsSuggestion: true, Side: domain.PageReviewCommentSideNew, StartLine: 3, EndLine: 3, Original: "three", Replacement: "THREE"},
		},
	}
	pages := NewPages(repository, slog.Default())

	_, err := pages.ApplyReviewSuggestion(context.Background(), 7, "guide", 10, domain.User{ID: 9, Role: "editor"})

	require.NoError(t, err)
	assert.Equal(t, []int64{10}, repository.appliedIDs)
	assert.False(t, repository.appliedAll)
	assert.Equal(t, "ONE\ntwo\nthree", repository.appliedMarkdown)
}

// TestValidateReviewCommentInputRejectsSuggestionOnOldSide verifies applicable suggestions target only reviewed-revision lines.
func TestValidateReviewCommentInputRejectsSuggestionOnOldSide(t *testing.T) {
	t.Parallel()

	err := validateReviewCommentInput(PageReviewCommentInput{
		ReviewID:    7,
		Slug:        "guide",
		Side:        domain.PageReviewCommentSideOld,
		StartLine:   1,
		EndLine:     1,
		Suggestion:  true,
		Replacement: "new",
	})

	validation, ok := err.(*domain.ValidationError)
	require.True(t, ok)
	assert.Contains(t, validation.Fields, domain.FieldError{Field: "suggestion", Message: "Suggestions can only replace lines in the reviewed revision."})
}

// TestReviewRangeVisibleRejectsSourceOutsideDisplayedHunks verifies forged line anchors cannot target hidden source.
func TestReviewRangeVisibleRejectsSourceOutsideDisplayedHunks(t *testing.T) {
	t.Parallel()

	before := make([]string, 20)
	after := make([]string, 20)
	for index := range before {
		before[index] = fmt.Sprintf("line %d", index+1)
		after[index] = before[index]
	}
	after[1] = "changed line 2"

	record := revision.Revision{
		Number:           2,
		PreviousMarkdown: strings.Join(before, "\n"),
		Markdown:         strings.Join(after, "\n"),
	}

	assert.True(t, reviewRangeVisible(record, domain.PageReviewCommentSideNew, 2, 2))
	assert.False(t, reviewRangeVisible(record, domain.PageReviewCommentSideNew, 20, 20))
}
