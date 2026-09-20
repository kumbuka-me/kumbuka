package pages

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type editorCatalogStub struct {
	page domain.Page
	err  error
}

func (s editorCatalogStub) GetPageForEdit(context.Context, domain.User, string) (domain.Page, error) {
	return s.page, s.err
}

type editorGroupStub struct{ groups []domain.Group }

func (s editorGroupStub) AssignableGroups(context.Context, domain.User) ([]domain.Group, error) {
	return s.groups, nil
}

type editorTemplateStub struct {
	templates []domain.PageTemplate
	selected  domain.PageTemplate
	err       error
}

func (s editorTemplateStub) PageTemplates(context.Context) ([]domain.PageTemplate, error) {
	return s.templates, nil
}

func (s editorTemplateStub) PageTemplate(context.Context, int64) (domain.PageTemplate, error) {
	return s.selected, s.err
}

func TestEditorLoadsExistingOrNewWorkflowData(t *testing.T) {
	t.Parallel()
	groups := []domain.Group{{ID: 1, Name: "Docs"}}
	templates := []domain.PageTemplate{{ID: 2, Name: "Runbook"}}
	selected := domain.PageTemplate{ID: 2, Name: "Runbook"}

	existing, err := NewEditor(
		editorCatalogStub{page: domain.Page{Slug: "guide", Title: "Guide"}},
		editorGroupStub{groups: groups},
		editorTemplateStub{templates: templates, selected: selected},
	).Load(context.Background(), domain.User{ID: 7}, "guide", 2)
	require.NoError(t, err)
	require.NotNil(t, existing.Page)
	assert.Equal(t, "guide", existing.Page.Slug)
	assert.Equal(t, groups, existing.Groups)
	assert.Nil(t, existing.Templates)
	assert.Nil(t, existing.SelectedTemplate)

	created, err := NewEditor(
		editorCatalogStub{},
		editorGroupStub{groups: groups},
		editorTemplateStub{templates: templates, selected: selected},
	).Load(context.Background(), domain.User{ID: 7}, "", 2)
	require.NoError(t, err)
	assert.Nil(t, created.Page)
	assert.Equal(t, groups, created.Groups)
	assert.Equal(t, templates, created.Templates)
	require.NotNil(t, created.SelectedTemplate)
	assert.Equal(t, int64(2), created.SelectedTemplate.ID)
}

type editorPageSaverStub struct {
	input PageSaveInput
	page  domain.Page
}

func (s *editorPageSaverStub) Save(_ context.Context, input PageSaveInput) (domain.Page, error) {
	s.input = input
	return s.page, nil
}

type editorDraftDiscarderStub struct {
	userID int64
	key    string
}

func (s *editorDraftDiscarderStub) Delete(_ context.Context, userID int64, key string) error {
	s.userID = userID
	s.key = key
	return nil
}

func TestEditorSaveMaterializesTemplateAndDiscardsDraft(t *testing.T) {
	t.Parallel()
	pages := &editorPageSaverStub{page: domain.Page{ID: 9, Slug: "guide"}}
	drafts := &editorDraftDiscarderStub{}
	templates := editorTemplateStub{selected: domain.PageTemplate{Fields: []domain.PageTemplateField{{
		Name: "owner", Label: "Owner", Required: true,
	}}}}
	command := NewEditorSave(pages, drafts, templates, nil)

	page, err := command.Execute(context.Background(), EditorSaveInput{
		Page: PageSaveInput{
			Slug:     "guide",
			Title:    "Guide",
			Markdown: "Owner: {{field:owner}}",
			Actor:    domain.User{ID: 7},
		},
		TemplateID:     2,
		TemplateValues: map[string]string{"owner": "Docs"},
	})
	require.NoError(t, err)
	assert.Equal(t, "guide", page.Slug)
	assert.Equal(t, "Owner: Docs", pages.input.Markdown)
	assert.Equal(t, int64(7), drafts.userID)
	assert.Equal(t, "new", drafts.key)
}

func TestEditorSaveRejectsMissingRequiredTemplateValueBeforePersistence(t *testing.T) {
	t.Parallel()
	pages := &editorPageSaverStub{}
	command := NewEditorSave(pages, &editorDraftDiscarderStub{}, editorTemplateStub{selected: domain.PageTemplate{Fields: []domain.PageTemplateField{{
		Name: "owner", Label: "Owner", Required: true,
	}}}}, nil)

	_, err := command.Execute(context.Background(), EditorSaveInput{
		Page:           PageSaveInput{Slug: "guide", Actor: domain.User{ID: 7}},
		TemplateID:     2,
		TemplateValues: map[string]string{},
	})

	validation, ok := err.(*domain.ValidationError)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "blueprint_owner", validation.Fields[0].Field)
	assert.Empty(t, pages.input.Slug)
}
