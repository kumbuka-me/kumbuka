package pages

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type searchRepositoryStub struct {
	taggedPages []domain.Page
}

func (searchRepositoryStub) ListPages(context.Context, int) ([]domain.Page, error) {
	return nil, nil
}

func (searchRepositoryStub) Search(context.Context, string, int) ([]domain.Page, error) {
	return nil, nil
}

func (s searchRepositoryStub) TaggedPages(context.Context) ([]domain.Page, error) {
	return s.taggedPages, nil
}

type searchAccessStub struct {
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
