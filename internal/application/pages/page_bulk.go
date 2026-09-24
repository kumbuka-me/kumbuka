package pages

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// ImportedPage contains one transport-independent page discovered by an importer.
type ImportedPage struct {
	// Slug is the source page path supplied by the importer.
	Slug string
	// Title is the imported page title.
	Title string
	// Markdown is the imported canonical source.
	Markdown string
	// Source describes the importer or source archive for revision history.
	Source string
}

// BulkPageInput contains one mutation to apply to a set of pages.
type BulkPageInput struct {
	// Action selects the bulk mutation to perform.
	Action string
	// Slugs identifies the selected pages.
	Slugs []string
	// Status is the replacement lifecycle status for a status action.
	Status string
	// Tag is added by a tag action.
	Tag string
	// GroupID is assigned by a group action.
	GroupID int64
	// Target is the destination parent path for a move action.
	Target string
	// Actor is the authenticated administrator performing the bulk mutation.
	Actor domain.User
}

// pageBulkRepository contains persistence operations that mutate sets of pages.
type pageBulkRepository interface {
	BulkAddPageTag(context.Context, []string, string) error
	BulkAssignPageGroup(context.Context, []string, int64) error
	BulkDeletePages(context.Context, []string, int64) error
	BulkMovePages(context.Context, []string, string, domain.User) error
	BulkSetPageStatus(context.Context, []string, string) error
}

// bulkRepository composes only persistence required by bulk and import workflows.
type bulkRepository interface {
	pageBulkRepository
	GetPage(context.Context, string) (domain.Page, error)
}

// Bulk owns administrative bulk mutations and imports.
type Bulk struct {
	// repository applies set-based page mutations and loads imported targets.
	repository bulkRepository
	// mutations owns single-page create and update rules reused by imports.
	mutations *Mutations
	// effects emits best-effort audit, notification, and webhook side effects.
	effects *pageEffects
}

// NewBulk constructs page bulk and import use cases.
func NewBulk(
	repository bulkRepository,
	mutations *Mutations,
	sideEffects pageSideEffectRepository,
	logger *slog.Logger,
	eventSinks ...webhooks.EventSink,
) *Bulk {
	return &Bulk{
		repository: repository,
		mutations:  mutations,
		effects:    newPageEffects(sideEffects, logger, eventSinks...),
	}
}

// Import persists imported pages while retaining workflow metadata on replacements.
func (s *Bulk) Import(ctx context.Context, candidates []ImportedPage, format string, actor domain.User) (int, error) {
	validated, err := validateImportedPages(candidates)
	if err != nil {
		return 0, err
	}

	for _, candidate := range validated {
		if err := s.importPage(ctx, candidate, actor); err != nil {
			return 0, err
		}
	}

	s.effects.recordAudit(
		ctx,
		actor.ID,
		"pages.imported",
		"import",
		format,
		fmt.Sprintf("Imported %d pages", len(candidates)),
	)

	return len(candidates), nil
}

// validateImportedPages rejects invalid and colliding canonical paths before any page is changed.
func validateImportedPages(candidates []ImportedPage) ([]ImportedPage, error) {
	validated := make([]ImportedPage, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))

	for _, candidate := range candidates {
		slug := md.Slug(candidate.Slug)
		validation := &domain.ValidationError{}
		validatePageSlug(slug, validation)
		if len(validation.Fields) != 0 {
			return nil, validation
		}
		if seen[slug] {
			return nil, domain.NewValidationError("slug", fmt.Sprintf("Multiple imported pages resolve to %q.", slug))
		}

		seen[slug] = true
		candidate.Slug = slug
		validated = append(validated, candidate)
	}

	return validated, nil
}

// importPage persists one import candidate while retaining existing metadata.
func (s *Bulk) importPage(ctx context.Context, candidate ImportedPage, actor domain.User) error {
	input := PageSaveInput{
		Slug:       candidate.Slug,
		Title:      candidate.Title,
		Markdown:   candidate.Markdown,
		Message:    "Imported from " + candidate.Source,
		Status:     "verified",
		Properties: map[string]string{},
		Actor:      actor,
	}
	current, err := s.repository.GetPage(ctx, candidate.Slug)

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

	_, err = s.mutations.save(ctx, input)

	return err
}

// Bulk applies one administrative mutation and records a single audit event.
func (s *Bulk) Bulk(ctx context.Context, input BulkPageInput) error {
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

	s.effects.recordAudit(
		ctx,
		input.Actor.ID,
		"page.bulk_"+input.Action,
		"page",
		strings.Join(input.Slugs, ","),
		fmt.Sprintf("%d pages", len(input.Slugs)),
	)

	return nil
}

// bulkMove validates the requested target and delegates the complete move set as one transaction.
func (s *Bulk) bulkMove(ctx context.Context, slugs []string, target string, actor domain.User) error {
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

	return s.repository.BulkMovePages(ctx, slugs, target, actor)
}
