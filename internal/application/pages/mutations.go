package pages

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
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

type pageUsageAnalyzer interface {
	AnalyzeUsage(string) pluginusage.Index
}

// Mutations coordinates core page mutations and their application-level side effects.
type Mutations struct {
	// repository persists core page content and revision mutations.
	repository pageContentRepository
	// authorization enforces resource-level page visibility and mutation permissions.
	authorization pageAuthorization
	// effects emits best-effort audit, mention, watcher, and webhook side effects.
	effects *pageEffects
	// usageAnalyzer derives plugin usage metadata from Markdown before persistence.
	usageAnalyzer pageUsageAnalyzer
	// renderer materializes stable HTML during writes when safe.
	renderer *md.Renderer
	// iconCatalog validates page icons against the active built-in and plugin catalog.
	iconCatalog *icons.Catalog
}

// NewMutations constructs core page mutation use cases. Event sinks are optional so
// page mutations remain independently testable.
func NewMutations(
	repository pageContentRepository,
	access appaccess.Policy,
	sideEffects pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *Mutations {
	if logger == nil {
		logger = slog.Default()
	}

	return &Mutations{
		repository:    repository,
		authorization: pageAuthorization{policy: access},
		effects:       newPageEffects(sideEffects, logger, eventSinks...),
		iconCatalog:   icons.Builtin(),
	}
}

// WithIconCatalog uses the active plugin-aware icon catalog for validation.
func (s *Mutations) WithIconCatalog(catalog *icons.Catalog) *Mutations {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	s.iconCatalog = catalog
	return s
}

// WithUsageAnalyzer derives plugin usage metadata for every persisted page write.
func (s *Mutations) WithUsageAnalyzer(analyzer pageUsageAnalyzer) *Mutations {
	s.usageAnalyzer = analyzer
	return s
}

// WithRenderer enables materialized HTML for pages whose render is independent
// of request-local permissions and mutable plugin resource data.
func (s *Mutations) WithRenderer(renderer *md.Renderer) *Mutations {
	s.renderer = renderer
	if renderer == nil {
		s.usageAnalyzer = nil
	} else {
		s.usageAnalyzer = renderer
	}

	return s
}

// Save validates and persists a page, then records audit and mention side effects.
func (s *Mutations) Save(ctx context.Context, input PageSaveInput) (domain.Page, error) {
	destination := md.Slug(input.Slug)
	if destination == "" && strings.TrimSpace(input.PreviousSlug) == "" {
		destination = md.Slug(input.Title)
	}
	if err := s.authorization.requireEdit(ctx, input.Actor, input.PreviousSlug); err != nil {
		return domain.Page{}, err
	}
	if err := s.authorization.requireEdit(ctx, input.Actor, destination); err != nil {
		return domain.Page{}, err
	}

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

	s.effects.recordAudit(ctx, input.Actor.ID, action, "page", page.Slug, page.Title)
	s.effects.notifyMentions(
		ctx,
		input.Actor.ID,
		input.Markdown,
		"Mention in "+page.Title,
		"/pages/"+page.Slug,
	)
	s.effects.notifyWatchers(ctx, input.Actor.ID, page.Slug, actionTitle(action, page.Title), "A watched page changed.", "/pages/"+page.Slug)

	return page, nil
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
func (s *Mutations) save(ctx context.Context, input PageSaveInput) (domain.Page, error) {
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
func (s *Mutations) derivePageContent(ctx context.Context, markdown string) (*pluginusage.Index, domain.PageRender, error) {
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
func (s *Mutations) materializeRender(ctx context.Context, source string, usage *pluginusage.Index) (domain.PageRender, error) {
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
func (s *Mutations) Delete(ctx context.Context, slug string, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}
	if err := s.repository.DeletePage(ctx, slug, actor.ID); err != nil {
		return err
	}

	s.effects.recordAudit(ctx, actor.ID, "page.deleted", "page", slug, "Moved page to recycle bin")
	s.effects.notifyWatchers(ctx, actor.ID, slug, "Page deleted: "+slug, "A watched page was moved to the recycle bin.", "/")

	return nil
}

// Move moves a page or subtree and records the action.
func (s *Mutations) Move(
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
	if err := s.authorization.requireEdit(ctx, actor, oldSlug); err != nil {
		return err
	}
	if err := s.authorization.requireEdit(ctx, actor, newSlug); err != nil {
		return err
	}
	if err := s.repository.MovePage(ctx, oldSlug, newSlug, options, actor); err != nil {
		return err
	}

	s.effects.recordAudit(ctx, actor.ID, "page.moved", "page", newSlug, oldSlug+" → "+newSlug)
	s.effects.notifyWatchers(ctx, actor.ID, oldSlug, "Page moved: "+oldSlug, "The watched page moved to "+newSlug+".", "/pages/"+newSlug)

	return nil
}

// Review records a completed documentation review and its audit event.
func (s *Mutations) Review(ctx context.Context, slug string, actor domain.User) error {
	slug = strings.TrimSpace(slug)
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return err
	}
	if err := s.repository.MarkPageReviewed(ctx, slug); err != nil {
		return err
	}

	s.effects.recordAudit(ctx, actor.ID, "page.reviewed", "page", slug, "Documentation review completed")
	s.effects.notifyWatchers(ctx, actor.ID, slug, "Page reviewed: "+slug, "A watched page was reviewed.", "/pages/"+slug)

	return nil
}

// RestoreRevision creates a new page revision from a persisted historical revision.
func (s *Mutations) RestoreRevision(ctx context.Context, slug string, number int, actor domain.User) (domain.Page, error) {
	if err := s.authorization.requireEdit(ctx, actor, slug); err != nil {
		return domain.Page{}, err
	}
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

	s.effects.recordAudit(
		ctx,
		actor.ID,
		"page.revision_restored",
		"page",
		page.Slug,
		"Restored revision "+fmt.Sprint(number),
	)
	s.effects.notifyWatchers(ctx, actor.ID, page.Slug, "Revision restored: "+page.Title, "A watched page restored an older revision.", "/pages/"+page.Slug)

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
