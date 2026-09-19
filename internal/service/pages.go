package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PageSaveInput contains transport-independent page mutation fields.
type PageSaveInput struct {
	// PreviousSlug identifies the existing page being edited; empty means create.
	PreviousSlug string
	// ExpectedUpdatedAt is the page timestamp observed when an editor opened an existing page.
	ExpectedUpdatedAt time.Time
	// Slug is the requested canonical page path.
	Slug string
	// Title is the human-readable page title.
	Title string
	// Icon names the icon used for page save input.
	Icon string
	// Language selects the PostgreSQL text-search configuration for the page.
	Language string
	// Markdown is the canonical source content.
	Markdown string
	// Message describes the revision for history.
	Message string
	// Tags replaces the page tag set.
	Tags []string
	// GroupIDs replaces the groups allowed to collaborate on the page.
	GroupIDs []int64
	// Status is the page lifecycle status.
	Status string
	// OwnerGroupID identifies the group accountable for the page; zero means none.
	OwnerGroupID int64
	// ReviewIntervalDays controls when the page becomes due for documentation review.
	ReviewIntervalDays int
	// MarkReviewed records this save as a completed documentation review.
	MarkReviewed bool
	// DeprecatedTarget optionally points readers to the replacement page.
	DeprecatedTarget string
	// Properties replaces the page metadata properties.
	Properties map[string]string
	// Actor is the authenticated user performing the mutation.
	Actor domain.User
}

// pageRepository composes the persistence capabilities used across page workflows.
type pageRepository interface {
	pageContentRepository
	pagePresenceRepository
	pageDiscussionRepository
	pageBulkRepository
	pageReviewRepository
	pageReviewDiscussionRepository
	pageSideEffectRepository
}

// pageContentRepository contains persistence used by core page mutations and revision restoration.
type pageContentRepository interface {
	DeletePage(context.Context, string, int64) error
	GetPage(context.Context, string) (domain.Page, error)
	MarkPageReviewed(context.Context, string) error
	MovePage(context.Context, string, string, domain.MovePageOptions, domain.User) error
	Revision(context.Context, string, int) (revision.Revision, error)
	LatestRevision(context.Context, string) (revision.Revision, int, error)
	SavePage(context.Context, string, string, string, string, string, string, string, []string, []string, []int64, domain.PageMetadata, map[string]string, domain.PageRender, domain.User) (domain.Page, error)
	SavePageIfUnchanged(context.Context, time.Time, string, string, string, string, string, string, string, []string, []string, []int64, domain.PageMetadata, map[string]string, domain.PageRender, domain.User) (domain.Page, error)
}

// pagePresenceRepository persists short-lived collaborative editor presence.
type pagePresenceRepository interface {
	TouchPageEditor(context.Context, string, int64) error
	LeavePageEditor(context.Context, string, int64) error
	PageEditors(context.Context, string, int64, time.Duration) ([]domain.PageEditorPresence, error)
}

type pageUsageAnalyzer interface {
	AnalyzeUsage(string) pluginusage.Index
}

// Pages coordinates page mutations and their application-level side effects.
type Pages struct {
	// repository provides the composed persistence capabilities used by page workflows.
	repository pageRepository
	// logger records diagnostics emitted by pages.
	logger *slog.Logger
	// eventSinks receive committed page events after persistence succeeds.
	eventSinks []EventSink
	// usageAnalyzer derives plugin usage metadata from Markdown before persistence.
	usageAnalyzer pageUsageAnalyzer
	// renderer materializes stable HTML during writes when safe.
	renderer *md.Renderer
	// iconCatalog validates page icons against the active built-in and plugin catalog.
	iconCatalog *icons.Catalog
}

// NewPages constructs the page application service. Event sinks are optional so
// page mutations remain independently testable.
func NewPages(repository pageRepository, logger *slog.Logger, eventSinks ...EventSink) *Pages {
	if logger == nil {
		logger = slog.Default()
	}

	return &Pages{repository: repository, logger: logger, eventSinks: eventSinks, iconCatalog: icons.Builtin()}
}

// WithIconCatalog uses the active plugin-aware icon catalog for validation.
func (s *Pages) WithIconCatalog(catalog *icons.Catalog) *Pages {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	s.iconCatalog = catalog
	return s
}

// WithUsageAnalyzer derives plugin usage metadata for every persisted page write.
func (s *Pages) WithUsageAnalyzer(analyzer pageUsageAnalyzer) *Pages {
	s.usageAnalyzer = analyzer
	return s
}

// WithRenderer enables materialized HTML for pages whose render is independent
// of request-local permissions and mutable plugin resource data.
func (s *Pages) WithRenderer(renderer *md.Renderer) *Pages {
	s.renderer = renderer
	if renderer == nil {
		s.usageAnalyzer = nil
	} else {
		s.usageAnalyzer = renderer
	}

	return s
}

// Save validates and persists a page, then records audit and mention side effects.
func (s *Pages) Save(ctx context.Context, input PageSaveInput) (domain.Page, error) {
	page, err := s.save(ctx, input)
	if err != nil {
		return domain.Page{}, err
	}

	action := "page.updated"

	switch {
	case input.PreviousSlug == "":
		action = "page.created"
	case strings.TrimSpace(input.PreviousSlug) != page.Slug:
		action = "page.renamed"
	}

	s.recordAudit(ctx, input.Actor.ID, action, "page", page.Slug, page.Title)
	s.notifyMentions(
		ctx,
		input.Actor.ID,
		input.Markdown,
		"Mention in "+page.Title,
		"/pages/"+page.Slug,
	)
	s.notifyWatchers(ctx, input.Actor.ID, page.Slug, actionTitle(action, page.Title), "A watched page changed.", "/pages/"+page.Slug)

	return page, nil
}

const pageEditorPresenceTTL = 90 * time.Second

// PageEditors returns other users with a recent editor heartbeat for one page.
func (s *Pages) PageEditors(ctx context.Context, slug string, excludeUserID int64) ([]domain.PageEditorPresence, error) {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return nil, domain.ErrNotFound
	}

	return s.repository.PageEditors(ctx, slug, excludeUserID, pageEditorPresenceTTL)
}

// TouchPageEditor refreshes one authenticated user's editor presence.
func (s *Pages) TouchPageEditor(ctx context.Context, slug string, actor domain.User) error {
	if actor.ID <= 0 {
		return domain.ErrForbidden
	}

	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return domain.ErrNotFound
	}

	return s.repository.TouchPageEditor(ctx, slug, actor.ID)
}

// LeavePageEditor clears one authenticated user's editor presence.
func (s *Pages) LeavePageEditor(ctx context.Context, slug string, actor domain.User) error {
	if actor.ID <= 0 {
		return domain.ErrForbidden
	}

	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return nil
	}

	return s.repository.LeavePageEditor(ctx, slug, actor.ID)
}

// validPageWorkflowSettings reports whether page lifecycle and review metadata are internally valid.
func validPageWorkflowSettings(input PageSaveInput) bool {
	if !domain.ValidPageStatus(input.Status) {
		return false
	}
	if !domain.ValidReviewIntervalDays(input.ReviewIntervalDays) {
		return false
	}

	return input.OwnerGroupID >= 0
}

// save validates and persists a page without emitting side effects.
func (s *Pages) save(ctx context.Context, input PageSaveInput) (domain.Page, error) {
	input.PreviousSlug = strings.TrimSpace(input.PreviousSlug)
	input.Slug = md.Slug(input.Slug)
	input.Title = strings.TrimSpace(input.Title)
	if input.Slug == "" && input.PreviousSlug == "" {
		input.Slug = md.Slug(input.Title)
	}
	input.Icon = strings.TrimSpace(input.Icon)
	input.Language = strings.TrimSpace(input.Language)
	input.DeprecatedTarget = md.Slug(input.DeprecatedTarget)
	validation := &domain.ValidationError{}

	if input.Slug == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "slug", Message: "A page path is required."})
	} else if strings.HasPrefix(input.Slug, "/") ||
		strings.HasSuffix(input.Slug, "/") ||
		strings.Contains(input.Slug, "//") {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "slug",
			Message: "Use a page path without leading, trailing, or repeated slashes.",
		})
	}
	if input.Title == "" {
		validation.Fields = append(validation.Fields, domain.FieldError{Field: "title", Message: "Title is required."})
	}
	if !s.iconCatalog.IsIcon(input.Icon) {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "icon",
			Message: "Choose an icon from the available icon catalog.",
		})
	}
	if input.Language != "" && !validContentLanguage(input.Language) {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "language",
			Message: "Choose a supported content language.",
		})
	}
	if !validPageWorkflowSettings(input) {
		validation.Fields = append(validation.Fields, domain.FieldError{
			Field:   "status",
			Message: "Choose valid page workflow settings.",
		})
	}

	if len(validation.Fields) > 0 {
		return domain.Page{}, validation
	}

	pluginUsage, render, err := s.derivePageContent(ctx, input.Markdown)
	if err != nil {
		return domain.Page{}, err
	}

	metadata := domain.PageMetadata{
		Status:             input.Status,
		OwnerGroupID:       input.OwnerGroupID,
		ReviewIntervalDays: input.ReviewIntervalDays,
		MarkReviewed:       input.MarkReviewed,
		DeprecatedTarget:   input.DeprecatedTarget,
		PluginUsage:        pluginUsage,
	}
	links := md.Links(input.Markdown)

	if input.PreviousSlug != "" && !input.ExpectedUpdatedAt.IsZero() {
		return s.repository.SavePageIfUnchanged(
			ctx,
			input.ExpectedUpdatedAt,
			input.PreviousSlug,
			input.Slug,
			input.Title,
			input.Icon,
			input.Language,
			input.Markdown,
			input.Message,
			input.Tags,
			links,
			input.GroupIDs,
			metadata,
			input.Properties,
			render,
			input.Actor,
		)
	}

	return s.repository.SavePage(
		ctx,
		input.PreviousSlug,
		input.Slug,
		input.Title,
		input.Icon,
		input.Language,
		input.Markdown,
		input.Message,
		input.Tags,
		links,
		input.GroupIDs,
		metadata,
		input.Properties,
		render,
		input.Actor,
	)
}

// derivePageContent computes plugin usage and the reusable render artifact for canonical Markdown.
func (s *Pages) derivePageContent(ctx context.Context, markdown string) (*pluginusage.Index, domain.PageRender, error) {
	var pluginUsage *pluginusage.Index
	if s.usageAnalyzer != nil {
		usage := s.usageAnalyzer.AnalyzeUsage(markdown)
		pluginUsage = &usage
	}

	render, err := s.materializeRender(ctx, markdown, pluginUsage)
	if err != nil {
		return nil, domain.PageRender{}, err
	}

	return pluginUsage, render, nil
}

// materializeRender renders stable page content once so normal GET requests can
// reuse it. Dynamic macro/substitution pages intentionally return an empty artifact.
func (s *Pages) materializeRender(ctx context.Context, source string, usage *pluginusage.Index) (domain.PageRender, error) {
	if s.renderer == nil || !s.renderer.CanPersist(source, usage) {
		return domain.PageRender{}, nil
	}
	options := md.DefaultOptions()
	rendered, err := s.renderer.RenderPageResolvedWithFunctions(source, md.Slug, options, md.Functions{
		Context:     ctx,
		PluginUsage: usage,
	})
	if err != nil {
		return domain.PageRender{}, err
	}
	// Inspector/export metadata originates from mutable substitutions. Keep such
	// pages on the request-time path as an additional persistence guard.
	if len(rendered.Inspectors) != 0 || len(rendered.ExportFields) != 0 {
		return domain.PageRender{}, nil
	}
	contents := make([]domain.PageHeading, len(rendered.Contents))
	for index, heading := range rendered.Contents {
		contents[index] = domain.PageHeading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}
	return domain.PageRender{
		HTML:        rendered.HTML,
		Contents:    contents,
		Fingerprint: s.renderer.RenderFingerprint(options),
	}, nil
}

// Delete moves a page to the recycle bin and records the action.
func (s *Pages) Delete(ctx context.Context, slug string, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.repository.DeletePage(ctx, slug, actor.ID); err != nil {
		return err
	}

	s.recordAudit(ctx, actor.ID, "page.deleted", "page", slug, "Moved page to recycle bin")
	s.notifyWatchers(ctx, actor.ID, slug, "Page deleted: "+slug, "A watched page was moved to the recycle bin.", "/")

	return nil
}

// Move moves a page or subtree and records the action.
func (s *Pages) Move(
	ctx context.Context,
	oldSlug, newSlug string,
	options domain.MovePageOptions,
	actor domain.User,
) error {
	oldSlug = strings.Trim(strings.TrimSpace(oldSlug), "/")
	newSlug = md.Slug(newSlug)
	if oldSlug == "" || newSlug == "" {
		return &domain.ValidationError{Fields: []domain.FieldError{{Field: "slug", Message: "A destination path is required."}}}
	}
	if oldSlug == newSlug {
		return domain.NewValidationError("slug", "Choose a different destination path.")
	}
	if options.MoveChildren && strings.HasPrefix(newSlug, oldSlug+"/") {
		return domain.NewValidationError("slug", "A page tree cannot be moved inside itself.")
	}
	if err := s.repository.MovePage(ctx, oldSlug, newSlug, options, actor); err != nil {
		return err
	}

	s.recordAudit(ctx, actor.ID, "page.moved", "page", newSlug, oldSlug+" → "+newSlug)
	s.notifyWatchers(ctx, actor.ID, oldSlug, "Page moved: "+oldSlug, "The watched page moved to "+newSlug+".", "/pages/"+newSlug)

	return nil
}

// Review records a completed documentation review and its audit event.
func (s *Pages) Review(ctx context.Context, slug string, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.repository.MarkPageReviewed(ctx, slug); err != nil {
		return err
	}

	s.recordAudit(ctx, actor.ID, "page.reviewed", "page", slug, "Documentation review completed")
	s.notifyWatchers(ctx, actor.ID, slug, "Page reviewed: "+slug, "A watched page was reviewed.", "/pages/"+slug)

	return nil
}

// RestoreRevision creates a new page revision from a persisted historical revision.
func (s *Pages) RestoreRevision(ctx context.Context, slug string, number int, actor domain.User) (domain.Page, error) {
	if number <= 0 {
		return domain.Page{}, &domain.ValidationError{Fields: []domain.FieldError{{Field: "revision", Message: "Invalid revision."}}}
	}

	page, err := s.repository.GetPage(ctx, slug)
	if err != nil {
		return domain.Page{}, err
	}

	record, err := s.repository.Revision(ctx, slug, number)
	if err != nil {
		return domain.Page{}, err
	}

	groupIDs := make([]int64, 0, len(page.Groups))

	for _, group := range page.Groups {
		groupIDs = append(groupIDs, group.ID)
	}

	properties := make(map[string]string, len(page.Properties))

	for _, property := range page.Properties {
		properties[property.Key] = property.Value
	}

	page, err = s.save(ctx, PageSaveInput{
		PreviousSlug:       page.Slug,
		Slug:               page.Slug,
		Title:              page.Title,
		Icon:               page.Icon,
		Language:           page.Language,
		Markdown:           record.Markdown,
		Message:            "Restore revision " + fmt.Sprint(number),
		Tags:               page.Tags,
		GroupIDs:           groupIDs,
		Status:             page.Status,
		OwnerGroupID:       page.OwnerGroupID,
		ReviewIntervalDays: page.ReviewIntervalDays,
		DeprecatedTarget:   page.DeprecatedTarget,
		Properties:         properties,
		Actor:              actor,
	})
	if err != nil {
		return domain.Page{}, err
	}

	s.recordAudit(
		ctx,
		actor.ID,
		"page.revision_restored",
		"page",
		page.Slug,
		"Restored revision "+fmt.Sprint(number),
	)
	s.notifyWatchers(ctx, actor.ID, page.Slug, "Revision restored: "+page.Title, "A watched page restored an older revision.", "/pages/"+page.Slug)

	return page, nil
}

// validContentLanguage reports whether value is supported by PostgreSQL search configuration.
func validContentLanguage(value string) bool {
	switch value {
	case "arabic", "chinese", "danish", "dutch", "english", "finnish", "french", "german", "greek",
		"hungarian", "italian", "japanese", "norwegian", "portuguese", "romanian", "russian", "spanish",
		"swedish", "turkish":
		return true
	default:
		return false
	}
}

// actionTitle returns the notification title for a page mutation event.
func actionTitle(action, title string) string {
	switch action {
	case "page.created":
		return "Page created: " + title
	case "page.renamed":
		return "Page renamed: " + title
	default:
		return "Page updated: " + title
	}
}
