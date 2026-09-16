package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// PageSaveInput contains transport-independent page mutation fields.
type PageSaveInput struct {
	PreviousSlug       string
	Slug               string
	Title              string
	Icon               string
	Language           string
	Markdown           string
	Message            string
	Tags               []string
	GroupIDs           []int64
	Status             string
	OwnerGroupID       int64
	ReviewIntervalDays int
	MarkReviewed       bool
	DeprecatedTarget   string
	Properties         map[string]string
	Actor              domain.User
}

// ImportedPage contains one transport-independent page discovered by an importer.
type ImportedPage struct {
	Slug     string
	Title    string
	Markdown string
	Source   string
}

// BulkPageInput contains one mutation to apply to a set of pages.
type BulkPageInput struct {
	Action  string
	Slugs   []string
	Status  string
	Tag     string
	GroupID int64
	Target  string
	Actor   domain.User
}

// PageReviewRequestInput contains the editable fields used to open a review.
type PageReviewRequestInput struct {
	Slug              string
	ReviewerUsernames []string
	ReviewerGroupID   int64
	Note              string
	Actor             domain.User
}

// PageReviewUpdateInput contains the editable fields of an existing pending review.
type PageReviewUpdateInput struct {
	ID                int64
	Slug              string
	ReviewerUsernames []string
	ReviewerGroupID   int64
	Note              string
	Actor             domain.User
}

// PageReviewDecisionInput contains one immutable decision for a pending review.
type PageReviewDecisionInput struct {
	ID       int64
	Slug     string
	Decision string
	Note     string
	Actor    domain.User
}

// pageRepository is the persistence contract required by page use cases.
// Keeping it here makes the application service independently testable and
// prevents unrelated store capabilities from becoming implicit dependencies.
type pageRepository interface {
	AddPageComment(context.Context, string, int64, string, string) (domain.PageComment, error)
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	BulkAddPageTag(context.Context, []string, string) error
	BulkAssignPageGroup(context.Context, []string, int64) error
	BulkDeletePages(context.Context, []string, int64) error
	BulkSetPageStatus(context.Context, []string, string) error
	DeletePage(context.Context, string, int64) error
	GetPage(context.Context, string) (domain.Page, error)
	LogAudit(context.Context, int64, string, string, string, string) error
	MarkPageReviewed(context.Context, string) error
	MovePage(context.Context, string, string, domain.MovePageOptions, domain.User) error
	NotifyMentions(context.Context, int64, string, string, string) error
	NotifyPageWatchers(context.Context, int64, string, string, string, string) error
	PageReviewRequest(context.Context, string) (domain.PageReviewRequest, error)
	PageReviewRequestByID(context.Context, int64, string) (domain.PageReviewRequest, error)
	ReviewUsers(context.Context, []string) ([]domain.User, error)
	ReviewGroup(context.Context, int64) (domain.Group, error)
	ReviewGroups(context.Context) ([]domain.Group, error)
	RequestPageReview(context.Context, string, int64, []int64, int64, string) (domain.PageReviewRequest, error)
	UpdatePageReview(context.Context, int64, string, int64, []int64, int64, string) (domain.PageReviewRequest, error)
	CancelPageReview(context.Context, int64, string, int64) (string, error)
	CanReviewPage(context.Context, string, int64) (bool, error)
	DecidePageReview(context.Context, int64, string, int64, bool, string, string) (string, error)
	ResolvePageComment(context.Context, int64, bool) error
	Revision(context.Context, string, int) (revision.Revision, error)
	SavePage(context.Context, string, string, string, string, string, string, string, []string, []string, []int64, domain.PageMetadata, map[string]string, domain.PageRender, domain.User) (domain.Page, error)
}

type pageUsageAnalyzer interface {
	AnalyzeUsage(string) pluginusage.Index
}

// Pages coordinates page mutations and their application-level side effects.
type Pages struct {
	repository    pageRepository
	logger        *slog.Logger
	eventSinks    []EventSink
	usageAnalyzer pageUsageAnalyzer
	renderer      *md.Renderer
	iconCatalog   *icons.Catalog
}

// NewPages constructs the page application service. Event sinks are optional so
// page mutations remain independently testable.
func NewPages(repository pageRepository, logger *slog.Logger, eventSinks ...EventSink) *Pages {
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
	s.usageAnalyzer = renderer
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
	validation := &ValidationError{}

	if input.Slug == "" {
		validation.Fields = append(validation.Fields, FieldError{Field: "slug", Message: "A page path is required."})
	} else if strings.HasPrefix(input.Slug, "/") ||
		strings.HasSuffix(input.Slug, "/") ||
		strings.Contains(input.Slug, "//") {
		validation.Fields = append(validation.Fields, FieldError{
			Field:   "slug",
			Message: "Use a page path without leading, trailing, or repeated slashes.",
		})
	}
	if input.Title == "" {
		validation.Fields = append(validation.Fields, FieldError{Field: "title", Message: "Title is required."})
	}
	if !s.iconCatalog.IsIcon(input.Icon) {
		validation.Fields = append(validation.Fields, FieldError{
			Field:   "icon",
			Message: "Choose an icon from the available icon catalog.",
		})
	}
	if input.Language != "" && !validContentLanguage(input.Language) {
		validation.Fields = append(validation.Fields, FieldError{
			Field:   "language",
			Message: "Choose a supported content language.",
		})
	}
	if !validPageWorkflowSettings(input) {
		validation.Fields = append(validation.Fields, FieldError{
			Field:   "status",
			Message: "Choose valid page workflow settings.",
		})
	}

	if len(validation.Fields) > 0 {
		return domain.Page{}, validation
	}

	var pluginUsage *pluginusage.Index
	if s.usageAnalyzer != nil {
		usage := s.usageAnalyzer.AnalyzeUsage(input.Markdown)
		pluginUsage = &usage
	}

	render, err := s.materializeRender(ctx, input.Markdown, pluginUsage)
	if err != nil {
		return domain.Page{}, err
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
		md.Links(input.Markdown),
		input.GroupIDs,
		domain.PageMetadata{
			Status:             input.Status,
			OwnerGroupID:       input.OwnerGroupID,
			ReviewIntervalDays: input.ReviewIntervalDays,
			MarkReviewed:       input.MarkReviewed,
			DeprecatedTarget:   input.DeprecatedTarget,
			PluginUsage:        pluginUsage,
		},
		input.Properties,
		render,
		input.Actor,
	)
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
		return &ValidationError{Fields: []FieldError{{Field: "slug", Message: "A destination path is required."}}}
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
		return domain.Page{}, &ValidationError{Fields: []FieldError{{Field: "revision", Message: "Invalid revision."}}}
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

// AddComment adds a discussion comment and emits mention notifications.
func (s *Pages) AddComment(ctx context.Context, slug, anchor, body string, actor domain.User) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.NewValidationError("body", "A comment is required.")
	}
	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.DiscussionsEnabled {
		return ErrDiscussionsDisabled
	}

	slug = strings.TrimSpace(slug)
	if _, err := s.repository.AddPageComment(ctx, slug, actor.ID, anchor, body); err != nil {
		return err
	}

	s.notifyMentions(ctx, actor.ID, body, "Mention in "+slug, "/pages/"+slug+"#comments")
	s.notifyWatchers(ctx, actor.ID, slug, "New comment: "+slug, "A watched page has a new discussion comment.", "/pages/"+slug+"#comments")
	s.recordAudit(ctx, actor.ID, "comment.created", "page", slug, "Page discussion comment created")

	return nil
}

// ResolveComment changes one discussion's resolution state.
func (s *Pages) ResolveComment(ctx context.Context, id int64, resolved bool) error {
	if id <= 0 {
		return &ValidationError{Fields: []FieldError{{Field: "comment", Message: "Invalid comment."}}}
	}

	settings, err := s.repository.ApplicationSettings(ctx)
	if err != nil {
		return err
	}
	if !settings.DiscussionsEnabled {
		return ErrDiscussionsDisabled
	}

	return s.repository.ResolvePageComment(ctx, id, resolved)
}

// Import persists imported pages while retaining workflow metadata on replacements.
func (s *Pages) Import(ctx context.Context, candidates []ImportedPage, format string, actor domain.User) (int, error) {
	for _, candidate := range candidates {
		if err := s.importPage(ctx, candidate, actor); err != nil {
			return 0, err
		}
	}

	s.recordAudit(
		ctx,
		actor.ID,
		"pages.imported",
		"import",
		format,
		fmt.Sprintf("Imported %d pages", len(candidates)),
	)

	return len(candidates), nil
}

// importPage persists one import candidate while retaining existing metadata.
func (s *Pages) importPage(ctx context.Context, candidate ImportedPage, actor domain.User) error {
	slug := md.Slug(candidate.Slug)
	if slug == "" {
		return domain.NewValidationError("slug", fmt.Sprintf("Invalid imported page path %q.", candidate.Slug))
	}

	input := PageSaveInput{
		Slug:       slug,
		Title:      candidate.Title,
		Markdown:   candidate.Markdown,
		Message:    "Imported from " + candidate.Source,
		Status:     "verified",
		Properties: map[string]string{},
		Actor:      actor,
	}
	current, err := s.repository.GetPage(ctx, slug)

	if err == nil {
		input.Icon = current.Icon
		input.Language = current.Language
		input.Tags = current.Tags
		input.Status = current.Status
		input.OwnerGroupID = current.OwnerGroupID
		input.ReviewIntervalDays = current.ReviewIntervalDays
		input.DeprecatedTarget = current.DeprecatedTarget
		input.GroupIDs = make([]int64, 0, len(current.Groups))

		for _, group := range current.Groups {
			input.GroupIDs = append(input.GroupIDs, group.ID)
		}
		for _, property := range current.Properties {
			input.Properties[property.Key] = property.Value
		}
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	_, err = s.save(ctx, input)

	return err
}

// Bulk applies one administrative mutation and records a single audit event.
func (s *Pages) Bulk(ctx context.Context, input BulkPageInput) error {
	if len(input.Slugs) == 0 {
		return domain.NewValidationError("pages", "Select at least one page.")
	}

	var err error

	switch input.Action {
	case "status":
		if !domain.ValidPageStatus(input.Status) {
			return domain.NewValidationError("status", "Choose a valid page status.")
		}
		err = s.repository.BulkSetPageStatus(ctx, input.Slugs, input.Status)
	case "tag":
		if strings.TrimSpace(input.Tag) == "" {
			return domain.NewValidationError("tag", "Enter a tag.")
		}
		err = s.repository.BulkAddPageTag(ctx, input.Slugs, input.Tag)
	case "group":
		if input.GroupID <= 0 {
			return domain.NewValidationError("group_id", "Choose a valid group.")
		}
		err = s.repository.BulkAssignPageGroup(ctx, input.Slugs, input.GroupID)
	case "move":
		err = s.bulkMove(ctx, input.Slugs, input.Target, input.Actor)
	case "delete":
		err = s.repository.BulkDeletePages(ctx, input.Slugs, input.Actor.ID)
	default:
		return domain.NewValidationError("action", "Choose a valid bulk action.")
	}

	if err != nil {
		return err
	}

	s.recordAudit(
		ctx,
		input.Actor.ID,
		"page.bulk_"+input.Action,
		"page",
		strings.Join(input.Slugs, ","),
		fmt.Sprintf("%d pages", len(input.Slugs)),
	)

	return nil
}

// bulkMove relocates selected pages beneath a normalized target path.
func (s *Pages) bulkMove(ctx context.Context, slugs []string, target string, actor domain.User) error {
	target = md.Slug(target)
	if target == "" {
		return domain.NewValidationError("target", "A target path is required.")
	}

	for _, slug := range slugs {
		source := strings.Trim(strings.TrimSpace(slug), "/")
		if source == "" || source == target+"/"+path.Base(source) {
			return domain.NewValidationError("target", "Choose a different destination for every selected page.")
		}
	}

	orderedSlugs := slices.Clone(slugs)

	slices.SortFunc(orderedSlugs, compareMoveSlugs)

	for _, slug := range orderedSlugs {
		destination := strings.Trim(target, "/") + "/" + path.Base(slug)
		if err := s.repository.MovePage(
			ctx,
			slug,
			destination,
			domain.MovePageOptions{UpdateIncomingLinks: true, KeepAliases: true},
			actor,
		); err != nil {
			return err
		}
	}

	return nil
}

// ErrDiscussionsDisabled indicates that page discussions are globally disabled.
var ErrDiscussionsDisabled = errors.New("page discussions are disabled")

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

// compareMoveSlugs puts longer paths first so descendants move before ancestors.
func compareMoveSlugs(left, right string) int {
	return cmp.Compare(len(right), len(left))
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

// PageReviewRequest returns the active review workflow item for a page.
func (s *Pages) PageReviewRequest(ctx context.Context, slug string) (domain.PageReviewRequest, error) {
	return s.repository.PageReviewRequest(ctx, strings.TrimSpace(slug))
}

// ReviewGroups returns collaboration groups that can be selected as review targets.
func (s *Pages) ReviewGroups(ctx context.Context) ([]domain.Group, error) {
	return s.repository.ReviewGroups(ctx)
}

// CanReview reports whether an editor is assigned to the current pending review.
func (s *Pages) CanReview(ctx context.Context, slug string, actor domain.User) (bool, error) {
	if actor.Role == "admin" || actor.ExternalAdmin {
		return true, nil
	}
	if actor.Role != "editor" {
		return false, nil
	}

	return s.repository.CanReviewPage(ctx, strings.TrimSpace(slug), actor.ID)
}

// CanManageReview reports whether the actor may edit or cancel the pending request.
func (s *Pages) CanManageReview(request domain.PageReviewRequest, actor domain.User) bool {
	if request.ID == 0 || request.Status != domain.PageReviewStatusPending {
		return false
	}

	return actor.Role == "admin" || actor.ExternalAdmin || request.RequestedBy == actor.ID
}

// RequestReview opens a review for the current page revision and moves the page to draft.
func (s *Pages) RequestReview(ctx context.Context, input PageReviewRequestInput) (domain.PageReviewRequest, error) {
	input.Slug = strings.TrimSpace(input.Slug)
	if input.Slug == "" {
		return domain.PageReviewRequest{}, domain.NewValidationError("slug", "A page path is required.")
	}
	if !canRequestReview(input.Actor) {
		return domain.PageReviewRequest{}, domain.ErrForbidden
	}

	active, err := s.repository.PageReviewRequest(ctx, input.Slug)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	if active.Status == domain.PageReviewStatusPending {
		return domain.PageReviewRequest{}, domain.ErrReviewPending
	}
	if active.Status == domain.PageReviewStatusChangesRequested {
		return domain.PageReviewRequest{}, domain.ErrReviewChangesRequired
	}

	reviewerIDs, reviewerGroupID, err := s.resolveReviewTargets(
		ctx,
		input.Slug,
		input.ReviewerUsernames,
		input.ReviewerGroupID,
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	request, err := s.repository.RequestPageReview(
		ctx,
		input.Slug,
		input.Actor.ID,
		reviewerIDs,
		reviewerGroupID,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	s.recordAudit(ctx, input.Actor.ID, "page.review_requested", "page", input.Slug, "Review requested for revision "+fmt.Sprint(request.RevisionNumber))
	s.notifyWatchers(ctx, input.Actor.ID, input.Slug, "review-requested", "Review requested", "/pages/"+input.Slug)

	return request, nil
}

// UpdateReview changes reviewers, reviewer group, or note without changing the requested revision.
func (s *Pages) UpdateReview(ctx context.Context, input PageReviewUpdateInput) (domain.PageReviewRequest, error) {
	if input.ID <= 0 {
		return domain.PageReviewRequest{}, domain.NewValidationError("review", "Choose a valid review request.")
	}
	input.Slug = strings.TrimSpace(input.Slug)
	if input.Slug == "" {
		return domain.PageReviewRequest{}, domain.NewValidationError("slug", "A page path is required.")
	}

	request, err := s.repository.PageReviewRequestByID(ctx, input.ID, input.Slug)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}
	if request.Status != domain.PageReviewStatusPending {
		return domain.PageReviewRequest{}, domain.ErrReviewClosed
	}
	if !s.CanManageReview(request, input.Actor) {
		return domain.PageReviewRequest{}, domain.ErrForbidden
	}

	reviewerIDs, reviewerGroupID, err := s.resolveReviewTargets(
		ctx,
		input.Slug,
		input.ReviewerUsernames,
		input.ReviewerGroupID,
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	updated, err := s.repository.UpdatePageReview(
		ctx,
		input.ID,
		input.Slug,
		input.Actor.ID,
		reviewerIDs,
		reviewerGroupID,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return domain.PageReviewRequest{}, err
	}

	s.recordAudit(ctx, input.Actor.ID, "page.review_updated", "page", input.Slug, "Pending review request updated")
	s.notifyWatchers(ctx, input.Actor.ID, input.Slug, "review-updated", "Review request updated", "/pages/"+input.Slug)

	return updated, nil
}

// CancelReview cancels a pending request without rewriting its history.
func (s *Pages) CancelReview(ctx context.Context, id int64, slug string, actor domain.User) error {
	if id <= 0 {
		return domain.NewValidationError("review", "Choose a valid review request.")
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return domain.NewValidationError("slug", "A page path is required.")
	}

	request, err := s.repository.PageReviewRequestByID(ctx, id, slug)
	if err != nil {
		return err
	}
	if request.Status != domain.PageReviewStatusPending {
		return domain.ErrReviewClosed
	}
	if !s.CanManageReview(request, actor) {
		return domain.ErrForbidden
	}

	resolvedSlug, err := s.repository.CancelPageReview(ctx, id, slug, actor.ID)
	if err != nil {
		return err
	}

	s.recordAudit(ctx, actor.ID, "page.review_canceled", "page", resolvedSlug, "Pending review request canceled")
	s.notifyWatchers(ctx, actor.ID, resolvedSlug, "review-canceled", "Review request canceled", "/pages/"+resolvedSlug)

	return nil
}

// DecideReview approves the requested revision or asks the author for changes.
func (s *Pages) DecideReview(ctx context.Context, input PageReviewDecisionInput) error {
	if input.ID <= 0 {
		return domain.NewValidationError("review", "Choose a valid review request.")
	}
	if input.Decision != domain.PageReviewStatusApproved && input.Decision != domain.PageReviewStatusChangesRequested {
		return domain.NewValidationError("decision", "Choose approve or request changes.")
	}

	allowed, err := s.CanReview(ctx, input.Slug, input.Actor)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}

	administrator := input.Actor.Role == "admin" || input.Actor.ExternalAdmin
	resolvedSlug, err := s.repository.DecidePageReview(
		ctx,
		input.ID,
		strings.TrimSpace(input.Slug),
		input.Actor.ID,
		administrator,
		input.Decision,
		strings.TrimSpace(input.Note),
	)
	if err != nil {
		return err
	}

	s.recordAudit(ctx, input.Actor.ID, "page.review_"+input.Decision, "page", resolvedSlug, strings.TrimSpace(input.Note))
	s.notifyWatchers(ctx, input.Actor.ID, resolvedSlug, "review-"+input.Decision, "Review "+strings.ReplaceAll(input.Decision, "_", " "), "/pages/"+resolvedSlug)

	return nil
}

// canRequestReview reports whether an actor may open a page review.
func canRequestReview(actor domain.User) bool {
	return actor.Role == "admin" || actor.Role == "editor" || actor.ExternalAdmin
}

// resolveReviewTargets validates selected people and the optional group and applies the owner-group fallback.
func (s *Pages) resolveReviewTargets(
	ctx context.Context,
	slug string,
	usernames []string,
	groupID int64,
) ([]int64, int64, error) {
	if groupID < 0 {
		return nil, 0, domain.NewValidationError("reviewer_group_id", "Choose a valid reviewer group.")
	}

	usernames = normalizeReviewerUsernames(usernames)
	reviewers, err := s.repository.ReviewUsers(ctx, usernames)
	if err != nil {
		return nil, 0, err
	}
	if len(reviewers) != len(usernames) || !validReviewers(reviewers) {
		return nil, 0, domain.NewValidationError("reviewers", "Choose enabled editors or administrators as reviewers.")
	}

	if groupID > 0 {
		if _, err := s.repository.ReviewGroup(ctx, groupID); errors.Is(err, domain.ErrNotFound) {
			return nil, 0, domain.NewValidationError("reviewer_group_id", "Choose an existing reviewer group.")
		} else if err != nil {
			return nil, 0, err
		}
	}

	if groupID == 0 && len(reviewers) == 0 {
		page, err := s.repository.GetPage(ctx, slug)
		if err != nil {
			return nil, 0, err
		}
		groupID = page.OwnerGroupID
	}

	ids := make([]int64, 0, len(reviewers))
	for _, reviewer := range reviewers {
		ids = append(ids, reviewer.ID)
	}

	return ids, groupID, nil
}

// validReviewers reports whether every selected account has a role that can decide reviews.
func validReviewers(reviewers []domain.User) bool {
	for _, reviewer := range reviewers {
		if reviewer.Role != "admin" && reviewer.Role != "editor" {
			return false
		}
	}

	return true
}

// normalizeReviewerUsernames trims mention markers, removes blanks, and keeps each username once.
func normalizeReviewerUsernames(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		value = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(value), "@"))
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, key)
	}

	return result
}
