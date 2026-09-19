package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// inlineSuggestionRepositoryStub captures inline suggestion reads and writes for service tests.
type inlineSuggestionRepositoryStub struct {
	// pageRepository supplies unused page-service methods so tests implement only the exercised boundary.
	pageRepository
	// settings controls whether discussions are available.
	settings domain.ApplicationSettings
	// page is the current page returned to the service.
	page domain.Page
	// latest is the latest persisted page revision.
	latest revision.Revision
	// latestCount is the number of persisted revisions for the page.
	latestCount int
	// comment is the inline suggestion returned by comment lookup.
	comment domain.PageComment
	// added captures the suggestion persisted during creation.
	added domain.PageCommentSuggestion
	// addedAnchor captures the selected rendered text persisted with a suggestion.
	addedAnchor string
	// expectedMarkdown captures the optimistic source snapshot used during suggestion creation.
	expectedMarkdown string
	// appliedMarkdown captures the new canonical Markdown written when applying a suggestion.
	appliedMarkdown string
}

// ApplicationSettings returns the configured discussion settings.
func (r *inlineSuggestionRepositoryStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return r.settings, nil
}

// GetPage returns the configured current page when its slug matches.
func (r *inlineSuggestionRepositoryStub) GetPage(_ context.Context, slug string) (domain.Page, error) {
	if slug != r.page.Slug {
		return domain.Page{}, domain.ErrNotFound
	}

	return r.page, nil
}

// LatestRevision returns the configured latest page revision.
func (r *inlineSuggestionRepositoryStub) LatestRevision(_ context.Context, slug string) (revision.Revision, int, error) {
	if slug != r.page.Slug {
		return revision.Revision{}, 0, domain.ErrNotFound
	}

	return r.latest, r.latestCount, nil
}

// AddPageSuggestion captures one inline suggestion and returns it as a persisted comment.
func (r *inlineSuggestionRepositoryStub) AddPageSuggestion(
	_ context.Context,
	slug string,
	_ int64,
	anchor, _ string,
	suggestion domain.PageCommentSuggestion,
	expectedMarkdown string,
) (domain.PageComment, error) {
	if slug != r.page.Slug {
		return domain.PageComment{}, domain.ErrNotFound
	}

	r.added = suggestion
	r.addedAnchor = anchor
	r.expectedMarkdown = expectedMarkdown

	return domain.PageComment{ID: 42, PageID: r.page.ID, Anchor: anchor, Suggestion: &suggestion}, nil
}

// PageComment returns the configured inline suggestion when its identifiers match.
func (r *inlineSuggestionRepositoryStub) PageComment(_ context.Context, slug string, id int64) (domain.PageComment, error) {
	if slug != r.page.Slug || id != r.comment.ID {
		return domain.PageComment{}, domain.ErrCommentNotFound
	}

	return r.comment, nil
}

// ApplyPageCommentSuggestion captures the canonical Markdown produced by suggestion application.
func (r *inlineSuggestionRepositoryStub) ApplyPageCommentSuggestion(
	_ context.Context,
	slug string,
	commentID, _ int64,
	markdown, _ string,
	_ []string,
	_ *pluginusage.Index,
	_ domain.PageRender,
) (domain.Page, error) {
	if slug != r.page.Slug || commentID != r.comment.ID {
		return domain.Page{}, domain.ErrCommentNotFound
	}

	r.appliedMarkdown = markdown
	updated := r.page
	updated.Markdown = markdown

	return updated, nil
}

// LogAudit accepts best-effort audit writes from the page service.
func (*inlineSuggestionRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

// NotifyPageWatchers accepts best-effort page-watch notifications from the page service.
func (*inlineSuggestionRepositoryStub) NotifyPageWatchers(context.Context, int64, string, string, string, string) error {
	return nil
}

// NotifyMentions accepts best-effort mention notifications from the page service.
func (*inlineSuggestionRepositoryStub) NotifyMentions(context.Context, int64, string, string, string) error {
	return nil
}

// TestAddSuggestionMapsSelectedText verifies a unique rendered-text selection is mapped to its exact Markdown byte range.
func TestAddSuggestionMapsSelectedText(t *testing.T) {
	t.Parallel()

	const source = "Before **selected text** after."
	repository := &inlineSuggestionRepositoryStub{
		settings:    domain.ApplicationSettings{DiscussionsEnabled: true},
		page:        domain.Page{ID: 7, Slug: "guide", Markdown: source},
		latest:      revision.Revision{Number: 3, Markdown: source},
		latestCount: 3,
	}
	pages := NewPages(repository, slog.Default())

	comment, err := pages.AddSuggestion(
		context.Background(),
		"guide",
		"selected text",
		"",
		"replacement",
		domain.User{ID: 9, Role: "viewer"},
	)

	require.NoError(t, err)
	assert.Equal(t, int64(42), comment.ID)
	assert.Equal(t, "selected text", repository.addedAnchor)
	assert.Equal(t, source, repository.expectedMarkdown)
	assert.Equal(t, 3, repository.added.RevisionNumber)
	assert.Equal(t, strings.Index(source, "selected text"), repository.added.StartByte)
	assert.Equal(t, strings.Index(source, "selected text")+len("selected text"), repository.added.EndByte)
	assert.Equal(t, "selected text", repository.added.Original)
	assert.Equal(t, "replacement", repository.added.Replacement)
}

// TestAddSuggestionRejectsAmbiguousSelection verifies repeated selected text cannot be silently attached to the wrong source range.
func TestAddSuggestionRejectsAmbiguousSelection(t *testing.T) {
	t.Parallel()

	const source = "same text, same text"
	repository := &inlineSuggestionRepositoryStub{
		settings:    domain.ApplicationSettings{DiscussionsEnabled: true},
		page:        domain.Page{ID: 7, Slug: "guide", Markdown: source},
		latest:      revision.Revision{Number: 2, Markdown: source},
		latestCount: 2,
	}

	_, err := NewPages(repository, slog.Default()).AddSuggestion(
		context.Background(),
		"guide",
		"same text",
		"",
		"changed",
		domain.User{ID: 9, Role: "viewer"},
	)

	validation, ok := errors.AsType[*ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "anchor", validation.Fields[0].Field)
	assert.Contains(t, validation.Fields[0].Message, "appears more than once")
}

// TestApplyInlineSuggestion verifies exact source ranges are replaced without disturbing surrounding Markdown.
func TestApplyInlineSuggestion(t *testing.T) {
	t.Parallel()

	updated, err := applyInlineSuggestion(
		"before selected after",
		domain.PageCommentSuggestion{StartByte: 7, EndByte: 15, Original: "selected", Replacement: "changed"},
	)

	require.NoError(t, err)
	assert.Equal(t, "before changed after", updated)
}

// TestApplyInlineSuggestionRejectsStaleSource verifies mismatched original text cannot be applied to a changed page.
func TestApplyInlineSuggestionRejectsStaleSource(t *testing.T) {
	t.Parallel()

	_, err := applyInlineSuggestion(
		"before modified after",
		domain.PageCommentSuggestion{StartByte: 7, EndByte: 15, Original: "selected", Replacement: "changed"},
	)

	assert.ErrorIs(t, err, domain.ErrStaleSuggestion)
}

// TestApplyCommentSuggestionPersistsNewRevisionSource verifies the service applies a current suggestion through the atomic repository path.
func TestApplyCommentSuggestionPersistsNewRevisionSource(t *testing.T) {
	t.Parallel()

	const source = "before selected after"
	suggestion := domain.PageCommentSuggestion{
		RevisionNumber: 4,
		StartByte:      7,
		EndByte:        15,
		Original:       "selected",
		Replacement:    "changed",
	}
	repository := &inlineSuggestionRepositoryStub{
		settings:    domain.ApplicationSettings{DiscussionsEnabled: true},
		page:        domain.Page{ID: 7, Slug: "guide", Markdown: source},
		latest:      revision.Revision{Number: 4, Markdown: source},
		latestCount: 4,
		comment:     domain.PageComment{ID: 42, Anchor: "selected", Suggestion: &suggestion},
	}

	page, err := NewPages(repository, slog.Default()).ApplyCommentSuggestion(
		context.Background(),
		"guide",
		42,
		domain.User{ID: 11, Role: "editor"},
	)

	require.NoError(t, err)
	assert.Equal(t, "before changed after", repository.appliedMarkdown)
	assert.Equal(t, repository.appliedMarkdown, page.Markdown)
}

// TestApplyCommentSuggestionRejectsStaleRevision verifies a suggestion cannot cross a page revision boundary.
func TestApplyCommentSuggestionRejectsStaleRevision(t *testing.T) {
	t.Parallel()

	const source = "before selected after"
	suggestion := domain.PageCommentSuggestion{
		RevisionNumber: 3,
		StartByte:      7,
		EndByte:        15,
		Original:       "selected",
		Replacement:    "changed",
	}
	repository := &inlineSuggestionRepositoryStub{
		settings:    domain.ApplicationSettings{DiscussionsEnabled: true},
		page:        domain.Page{ID: 7, Slug: "guide", Markdown: source},
		latest:      revision.Revision{Number: 4, Markdown: source},
		latestCount: 4,
		comment:     domain.PageComment{ID: 42, Anchor: "selected", Suggestion: &suggestion},
	}

	_, err := NewPages(repository, slog.Default()).ApplyCommentSuggestion(
		context.Background(),
		"guide",
		42,
		domain.User{ID: 11, Role: "editor"},
	)

	assert.ErrorIs(t, err, domain.ErrStaleSuggestion)
	assert.Empty(t, repository.appliedMarkdown)
}
