package pages

import (
	"context"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PageContentUpdateInput replaces only the Markdown body of an existing page.
// Metadata is loaded from the current page and preserved by the mutation.
type PageContentUpdateInput struct {
	// Slug identifies the existing page to update.
	Slug string
	// Markdown is the replacement page body.
	Markdown string
	// Message describes the change in revision history.
	Message string
	// ExpectedUpdatedAt is the version timestamp used to reject stale writes.
	ExpectedUpdatedAt time.Time
	// Actor is the user whose edit permissions apply to this operation.
	Actor domain.User
}

// UpdateContent performs a guarded page-body update while preserving all page metadata and running the normal revision, authorization, and side-effect path.
func (s *Mutations) UpdateContent(ctx context.Context, input PageContentUpdateInput) (domain.Page, error) {
	slug := strings.TrimSpace(input.Slug)
	if slug == "" || input.ExpectedUpdatedAt.IsZero() {
		return domain.Page{}, domain.NewValidationError("page", "A page and expected version are required.")
	}
	if err := s.authorization.requireEdit(ctx, input.Actor, slug); err != nil {
		return domain.Page{}, err
	}
	page, err := s.repository.GetPage(ctx, slug)
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

	return s.Save(ctx, PageSaveInput{
		PreviousSlug:       page.Slug,
		ExpectedUpdatedAt:  input.ExpectedUpdatedAt,
		Slug:               page.Slug,
		Title:              page.Title,
		Icon:               page.Icon,
		Language:           page.Language,
		Markdown:           input.Markdown,
		Message:            strings.TrimSpace(input.Message),
		Tags:               page.Tags,
		GroupIDs:           groupIDs,
		Status:             page.Status,
		OwnerGroupID:       page.OwnerGroupID,
		ReviewIntervalDays: page.ReviewIntervalDays,
		DeprecatedTarget:   page.DeprecatedTarget,
		Properties:         properties,
		Actor:              input.Actor,
	})
}
