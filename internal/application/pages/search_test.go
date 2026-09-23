package pages

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// searchRepositoryStub provides controllable search repository behavior for tests.
type searchRepositoryStub struct {
	// pages records the pages observed by the test double.
	pages []domain.Page
	// taggedPages configures or records the tagged pages value used by the fixture.
	taggedPages []domain.Page
}

func (s searchRepositoryStub) ListPagesPage(_ context.Context, limit, offset int) ([]domain.Page, error) {
	return pageWindow(s.pages, limit, offset), nil
}

func (s searchRepositoryStub) SearchPage(_ context.Context, _ string, limit, offset int) ([]domain.Page, error) {
	return pageWindow(s.pages, limit, offset), nil
}

func pageWindow(pages []domain.Page, limit, offset int) []domain.Page {
	if offset >= len(pages) {
		return nil
	}

	end := min(offset+limit, len(pages))
	return append([]domain.Page(nil), pages[offset:end]...)
}

func (s searchRepositoryStub) TaggedPages(context.Context) ([]domain.Page, error) {
	return s.taggedPages, nil
}

// searchAccessStub provides controllable search access behavior for tests.
type searchAccessStub struct {
	// visible configures or records the visible value used by the fixture.
	visible []domain.Page
}

func (*searchAccessStub) CanView(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (*searchAccessStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (s *searchAccessStub) FilterPages(_ context.Context, _ domain.User, _ []domain.Page) ([]domain.Page, error) {
	return append([]domain.Page(nil), s.visible...), nil
}

func TestTagsForExcludesRestrictedPageMetadata(t *testing.T) {
	t.Parallel()

	repository := searchRepositoryStub{taggedPages: []domain.Page{
		{Slug: "public", Tags: []string{"docs", "platform"}},
		{Slug: "secret", Tags: []string{"acquisition", "platform"}},
	}}
	access := &searchAccessStub{visible: []domain.Page{
		{Slug: "public", Tags: []string{"docs", "platform"}},
	}}

	tags, err := NewSearch(repository, access).TagsFor(context.Background(), domain.User{ID: 42})

	require.NoError(t, err)
	assert.Equal(t, []string{"docs", "platform"}, tags)
}

func TestSearchForFillsLimitAfterAccessFiltering(t *testing.T) {
	t.Parallel()

	pages := make([]domain.Page, 52)
	pages[0] = domain.Page{Slug: "restricted"}
	for index := 1; index < len(pages); index++ {
		pages[index] = domain.Page{Slug: "visible"}
	}

	access := &filteringSearchAccessStub{}
	result, err := NewSearch(searchRepositoryStub{pages: pages}, access).SearchFor(
		context.Background(),
		domain.User{ID: 42},
		"query",
		50,
	)

	require.NoError(t, err)
	require.Len(t, result, 50)
	assert.Equal(t, "visible", result[0].Slug)
	assert.Equal(t, 2, access.calls)
}

// filteringSearchAccessStub provides controllable filtering search access behavior for tests.
type filteringSearchAccessStub struct {
	// calls counts calls observed by the test double.
	calls int
}

func (*filteringSearchAccessStub) CanView(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (*filteringSearchAccessStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return false, nil
}

func (s *filteringSearchAccessStub) FilterPages(_ context.Context, _ domain.User, pages []domain.Page) ([]domain.Page, error) {
	s.calls++
	visible := make([]domain.Page, 0, len(pages))
	for _, page := range pages {
		if page.Slug != "restricted" {
			visible = append(visible, page)
		}
	}

	return visible, nil
}
