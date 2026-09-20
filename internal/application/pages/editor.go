package pages

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// editorCatalog loads pages through the actor-aware edit boundary.
type editorCatalog interface {
	GetPageForEdit(context.Context, domain.User, string) (domain.Page, error)
}

// editorGroupReader loads groups the current actor may assign to a page.
type editorGroupReader interface {
	AssignableGroups(context.Context, domain.User) ([]domain.Group, error)
}

// editorTemplateReader loads reusable page templates used by the editor workflow.
type editorTemplateReader interface {
	PageTemplates(context.Context) ([]domain.PageTemplate, error)
	PageTemplate(context.Context, int64) (domain.PageTemplate, error)
}

// editorPageSaver persists the transport-independent page input produced by the editor.
type editorPageSaver interface {
	Save(context.Context, PageSaveInput) (domain.Page, error)
}

// editorDraftDiscarder removes the private draft superseded by a successful save.
type editorDraftDiscarder interface {
	Delete(context.Context, int64, string) error
}

// Editor loads all application data required to open the create or edit workflow.
type Editor struct {
	catalog   editorCatalog
	groups    editorGroupReader
	templates editorTemplateReader
}

// EditorResult contains transport-independent data needed by the page editor.
type EditorResult struct {
	// Page is the existing editable page; nil indicates a new-page workflow.
	Page *domain.Page
	// Groups contains groups the actor may assign to the page.
	Groups []domain.Group
	// Templates contains available templates for a new page.
	Templates []domain.PageTemplate
	// SelectedTemplate is the requested new-page template when it exists.
	SelectedTemplate *domain.PageTemplate
}

// NewEditor constructs the page editor query from focused application readers.
func NewEditor(catalog editorCatalog, groups editorGroupReader, templates editorTemplateReader) *Editor {
	return &Editor{catalog: catalog, groups: groups, templates: templates}
}

// Load returns the application data for editing an existing page or creating a new one.
func (q *Editor) Load(
	ctx context.Context,
	actor domain.User,
	slug string,
	selectedTemplateID int64,
) (EditorResult, error) {
	groups, err := q.groups.AssignableGroups(ctx, actor)
	if err != nil {
		return EditorResult{}, err
	}

	result := EditorResult{Groups: groups}
	if strings.TrimSpace(slug) != "" {
		page, err := q.catalog.GetPageForEdit(ctx, actor, slug)
		if err != nil {
			return EditorResult{}, err
		}
		result.Page = &page
		return result, nil
	}

	result.Templates, err = q.templates.PageTemplates(ctx)
	if err != nil {
		return EditorResult{}, err
	}
	if selectedTemplateID <= 0 {
		return result, nil
	}

	selected, err := q.templates.PageTemplate(ctx, selectedTemplateID)
	if errors.Is(err, domain.ErrNotFound) {
		return result, nil
	}
	if err != nil {
		return EditorResult{}, err
	}
	result.SelectedTemplate = &selected
	return result, nil
}

// EditorSaveInput contains the page and template values submitted by the browser editor.
type EditorSaveInput struct {
	// Page contains the transport-independent page mutation fields.
	Page PageSaveInput
	// TemplateID identifies the optional template applied while creating a page.
	TemplateID int64
	// TemplateValues maps template field names to submitted values.
	TemplateValues map[string]string
}

// EditorSave coordinates template materialization, page persistence, and draft cleanup.
type EditorSave struct {
	pages     editorPageSaver
	drafts    editorDraftDiscarder
	templates editorTemplateReader
	logger    *slog.Logger
}

// NewEditorSave constructs the browser-editor save use case.
func NewEditorSave(
	pages editorPageSaver,
	drafts editorDraftDiscarder,
	templates editorTemplateReader,
	logger *slog.Logger,
) *EditorSave {
	if logger == nil {
		logger = slog.Default()
	}
	return &EditorSave{pages: pages, drafts: drafts, templates: templates, logger: logger}
}

// Execute saves one editor submission and discards the superseded private draft.
func (c *EditorSave) Execute(ctx context.Context, input EditorSaveInput) (domain.Page, error) {
	if strings.TrimSpace(input.Page.PreviousSlug) == "" && input.TemplateID > 0 {
		markdown, err := c.materializeTemplate(ctx, input.TemplateID, input.TemplateValues, input.Page.Markdown)
		if err != nil {
			return domain.Page{}, err
		}
		input.Page.Markdown = markdown
	}

	page, err := c.pages.Save(ctx, input.Page)
	if err != nil {
		return domain.Page{}, err
	}

	draftKey := "new"
	if strings.TrimSpace(input.Page.PreviousSlug) != "" {
		draftKey = PageDraftKey(page.ID)
	}
	if err := c.drafts.Delete(ctx, input.Page.Actor.ID, draftKey); err != nil {
		c.logger.WarnContext(ctx,
			"discard saved page draft",
			"event", "page_draft_cleanup_failed",
			"draft_key", draftKey,
			"user_id", input.Page.Actor.ID,
			"error", err,
		)
	}

	return page, nil
}

// materializeTemplate validates required values and substitutes template fields into Markdown.
func (c *EditorSave) materializeTemplate(
	ctx context.Context,
	templateID int64,
	values map[string]string,
	markdown string,
) (string, error) {
	template, err := c.templates.PageTemplate(ctx, templateID)
	if err != nil {
		return "", err
	}

	validation := &domain.ValidationError{}
	for _, field := range template.Fields {
		value := values[field.Name]
		if field.Required && strings.TrimSpace(value) == "" {
			validation.Fields = append(validation.Fields, domain.FieldError{
				Field:   "blueprint_" + field.Name,
				Message: field.Label + " is required.",
			})
		}
		markdown = strings.ReplaceAll(markdown, "{{field:"+field.Name+"}}", value)
	}
	if len(validation.Fields) > 0 {
		return "", validation
	}
	return markdown, nil
}
