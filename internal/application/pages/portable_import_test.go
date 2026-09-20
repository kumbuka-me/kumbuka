package pages

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// portableImportRepositoryStub records the page mutation produced by portable import.
type portableImportRepositoryStub struct {
	pageRepository
	// Existing reports whether GetPage should return an existing target page.
	Existing bool
	// PreviousSlug records the previous slug supplied to SavePage.
	PreviousSlug string
	// Slug records the canonical target slug.
	Slug string
	// Title records the imported page title.
	Title string
	// Icon records the imported icon identifier.
	Icon string
	// Language records the imported page language.
	Language string
	// Markdown records the imported Markdown body.
	Markdown string
	// Tags records imported page tags.
	Tags []string
	// GroupIDs records imported collaboration group identifiers.
	GroupIDs []int64
	// Metadata records imported workflow metadata.
	Metadata domain.PageMetadata
	// Properties records imported structured properties.
	Properties map[string]string
}

// GetPage reports whether the target page already exists.
func (r *portableImportRepositoryStub) GetPage(context.Context, string) (domain.Page, error) {
	if !r.Existing {
		return domain.Page{}, domain.ErrNotFound
	}
	return domain.Page{Slug: "guide"}, nil
}

// SavePage records the portable page mutation and returns its target page.
func (r *portableImportRepositoryStub) SavePage(
	_ context.Context,
	previousSlug, slug, title, icon, language, markdown, _ string,
	tags, _ []string,
	groupIDs []int64,
	metadata domain.PageMetadata,
	properties map[string]string,
	_ domain.PageRender,
	_ domain.User,
) (domain.Page, error) {
	r.PreviousSlug = previousSlug
	r.Slug = slug
	r.Title = title
	r.Icon = icon
	r.Language = language
	r.Markdown = markdown
	r.Tags = tags
	r.GroupIDs = groupIDs
	r.Metadata = metadata
	r.Properties = properties
	return domain.Page{Slug: slug, Title: title}, nil
}

func TestImportPortablePageRestoresArchiveMetadata(t *testing.T) {
	t.Parallel()

	repository := &portableImportRepositoryStub{}
	pages := NewPages(repository, nil, nil)

	err := pages.importPortablePage(context.Background(), PortableImportedPage{
		Slug:               "guide",
		Title:              "Guide",
		Language:           "german",
		Markdown:           "# Guide\n",
		Tags:               []string{"docs", "ops"},
		GroupIDs:           []int64{4, 9},
		Status:             "verified",
		OwnerGroupID:       9,
		ReviewIntervalDays: 180,
		DeprecatedTarget:   "guide-v2",
		Properties:         map[string]string{"owner": "platform"},
	}, domain.User{ID: 7})

	require.NoError(t, err)
	assert.Empty(t, repository.PreviousSlug)
	assert.Equal(t, "guide", repository.Slug)
	assert.Equal(t, "Guide", repository.Title)
	assert.Equal(t, "german", repository.Language)
	assert.Equal(t, "# Guide\n", repository.Markdown)
	assert.Equal(t, []string{"docs", "ops"}, repository.Tags)
	assert.Equal(t, []int64{4, 9}, repository.GroupIDs)
	assert.Equal(t, "verified", repository.Metadata.Status)
	assert.EqualValues(t, 9, repository.Metadata.OwnerGroupID)
	assert.Equal(t, 180, repository.Metadata.ReviewIntervalDays)
	assert.Equal(t, "guide-v2", repository.Metadata.DeprecatedTarget)
	assert.Equal(t, map[string]string{"owner": "platform"}, repository.Properties)
}

func TestImportPortablePageReplacesExistingMetadata(t *testing.T) {
	t.Parallel()

	repository := &portableImportRepositoryStub{Existing: true}
	pages := NewPages(repository, nil, nil)

	err := pages.importPortablePage(context.Background(), PortableImportedPage{
		Slug:       "guide",
		Title:      "Imported Guide",
		Markdown:   "Imported body",
		Status:     "draft",
		Properties: map[string]string{},
	}, domain.User{ID: 7})

	require.NoError(t, err)
	assert.Equal(t, "guide", repository.PreviousSlug)
	assert.Equal(t, "Imported Guide", repository.Title)
	assert.Equal(t, "draft", repository.Metadata.Status)
}
