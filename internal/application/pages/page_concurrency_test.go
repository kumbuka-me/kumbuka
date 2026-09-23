package pages

import (
	"context"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pageConcurrencyRepositoryStub records whether a page save used the guarded persistence path.
type pageConcurrencyRepositoryStub struct {
	pageContentRepository
	// guarded controls or records whether guarded is active in the test.
	guarded bool
	// unguarded controls or records whether unguarded is active in the test.
	unguarded bool
	// expectedUpdatedAt holds the updated at expected by the test.
	expectedUpdatedAt time.Time
}

// SavePage records an unconditional page save.
func (r *pageConcurrencyRepositoryStub) SavePage(
	_ context.Context,
	_, slug, title, _, _, _, _ string,
	_, _ []string,
	_ []int64,
	_ domain.PageMetadata,
	_ map[string]string,
	_ domain.PageRender,
	_ domain.User,
) (domain.Page, error) {
	r.unguarded = true
	return domain.Page{Slug: slug, Title: title}, nil
}

// SavePageIfUnchanged records a guarded page save and its expected page timestamp.
func (r *pageConcurrencyRepositoryStub) SavePageIfUnchanged(
	_ context.Context,
	expectedUpdatedAt time.Time,
	_, slug, title, _, _, _, _ string,
	_, _ []string,
	_ []int64,
	_ domain.PageMetadata,
	_ map[string]string,
	_ domain.PageRender,
	_ domain.User,
) (domain.Page, error) {
	r.guarded = true
	r.expectedUpdatedAt = expectedUpdatedAt
	return domain.Page{Slug: slug, Title: title}, nil
}

func TestSaveUsesOptimisticConcurrencyForEditorUpdates(t *testing.T) {
	t.Parallel()

	expected := time.Date(2026, time.September, 19, 14, 30, 0, 123000000, time.UTC)
	repository := &pageConcurrencyRepositoryStub{}
	mutations := NewMutations(repository, nil, nil, nil)

	_, err := mutations.save(context.Background(), PageSaveInput{
		PreviousSlug:      "guide",
		ExpectedUpdatedAt: expected,
		Slug:              "guide",
		Title:             "Guide",
		Markdown:          "Updated content",
		Status:            "verified",
	})

	require.NoError(t, err)
	assert.True(t, repository.guarded)
	assert.False(t, repository.unguarded)
	assert.True(t, repository.expectedUpdatedAt.Equal(expected))
}

func TestSaveKeepsInternalWritesUnconditional(t *testing.T) {
	t.Parallel()

	repository := &pageConcurrencyRepositoryStub{}
	mutations := NewMutations(repository, nil, nil, nil)

	_, err := mutations.save(context.Background(), PageSaveInput{
		PreviousSlug: "guide",
		Slug:         "guide",
		Title:        "Guide",
		Markdown:     "Imported content",
		Status:       "verified",
	})

	require.NoError(t, err)
	assert.False(t, repository.guarded)
	assert.True(t, repository.unguarded)
}
