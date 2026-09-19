package service

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type draftRepositoryStub struct {
	savedUserID int64
	savedKey    string
	savedPageID int64
	savedTitle  string
	savedSlug   string
	savedValues map[string][]string
	deletedKey  string
}

func (r *draftRepositoryStub) PageDraft(context.Context, int64, string) (domain.PageDraft, error) {
	return domain.PageDraft{}, domain.ErrNotFound
}

func (r *draftRepositoryStub) PageDrafts(context.Context, int64, int) ([]domain.PageDraft, error) {
	return nil, nil
}

func (r *draftRepositoryStub) SavePageDraft(
	_ context.Context,
	userID int64,
	key string,
	pageID int64,
	title string,
	slug string,
	values map[string][]string,
) (domain.PageDraft, error) {
	r.savedUserID = userID
	r.savedKey = key
	r.savedPageID = pageID
	r.savedTitle = title
	r.savedSlug = slug
	r.savedValues = values

	return domain.PageDraft{Key: key, PageID: pageID, Title: title, Slug: slug, Values: values}, nil
}

func (r *draftRepositoryStub) DeletePageDraft(_ context.Context, _ int64, key string) error {
	r.deletedKey = key
	return nil
}

func TestPageDraftKey(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "new", PageDraftKey(0))
	assert.Equal(t, "page:42", PageDraftKey(42))
}

func TestDraftsSaveValidatesStableKey(t *testing.T) {
	t.Parallel()

	_, err := NewDrafts(nil).Save(context.Background(), PageDraftSaveInput{
		Key:    "page:41",
		PageID: 42,
		Actor:  domain.User{ID: 7},
	})

	validation, ok := err.(*domain.ValidationError)

	require.True(t, ok)
	assert.Equal(t, "draft", validation.Fields[0].Field)
}

func TestDraftsSavePersistsPrivateFormState(t *testing.T) {
	t.Parallel()

	repository := &draftRepositoryStub{}
	drafts := NewDrafts(repository)
	values := map[string][]string{"title": {" Draft title "}, "group_id": {"2", "7"}}

	draft, err := drafts.Save(context.Background(), PageDraftSaveInput{
		Key:    "page:42",
		PageID: 42,
		Title:  " Draft title ",
		Slug:   " guide/new-path ",
		Values: values,
		Actor:  domain.User{ID: 7},
	})

	require.NoError(t, err)
	assert.Equal(t, int64(7), repository.savedUserID)
	assert.Equal(t, "page:42", repository.savedKey)
	assert.Equal(t, int64(42), repository.savedPageID)
	assert.Equal(t, "Draft title", repository.savedTitle)
	assert.Equal(t, "guide/new-path", repository.savedSlug)
	assert.Equal(t, values, repository.savedValues)
	assert.Equal(t, "page:42", draft.Key)
}

func TestDraftsDeleteIgnoresInvalidKeys(t *testing.T) {
	t.Parallel()

	repository := &draftRepositoryStub{}
	err := NewDrafts(repository).Delete(context.Background(), 7, "not-a-draft")

	require.NoError(t, err)
	assert.Empty(t, repository.deletedKey)
}
