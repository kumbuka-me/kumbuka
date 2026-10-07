package pages

import (
	"context"
	"errors"
	"fmt"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// PortableImportedPage contains one page reconstructed from a Kumbuka portable archive.
type PortableImportedPage struct {
	// Slug is the canonical page path.
	Slug string
	// Title is the human-readable page title.
	Title string
	// Icon is the optional icon identifier displayed beside the page title.
	Icon string
	// Language optionally overrides the instance-wide content language.
	Language string
	// Markdown is the page Markdown after archived resource references are restored.
	Markdown string
	// Tags contains normalized page tags.
	Tags []string
	// GroupIDs contains target-instance collaboration group identifiers.
	GroupIDs []int64
	// Status is the page lifecycle state.
	Status domain.PageStatus
	// OwnerGroupID identifies the target-instance owner group.
	OwnerGroupID int64
	// ReviewIntervalDays configures the page's documentation review cadence.
	ReviewIntervalDays int
	// DeprecatedTarget points a deprecated page at its replacement page path.
	DeprecatedTarget string
	// Properties contains searchable structured page metadata.
	Properties map[string]string
}

// PortableRepository contains persistence needed while restoring portable pages.
type PortableRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	PortablePageSaveRepository
}

// PortableImporter saves portable archive pages through one explicit repository scope.
type PortableImporter struct {
	// repository is the transaction-scoped persistence used by the import.
	repository PortableRepository
	// mutations provides shared page validation and derived-content preparation.
	mutations *Mutations
}

// NewPortableImporter constructs a portable page importer around an explicit repository scope.
func NewPortableImporter(repository PortableRepository, mutations *Mutations) *PortableImporter {
	return &PortableImporter{repository: repository, mutations: mutations}
}

// ImportPortablePages saves archive pages without side effects through the importer's explicit repository.
func (s *PortableImporter) ImportPortablePages(
	ctx context.Context,
	candidates []PortableImportedPage,
	actor domain.User,
) (int, error) {
	for index, candidate := range candidates {
		if err := s.importPortablePage(ctx, candidate, actor); err != nil {
			return index, err
		}
	}
	return len(candidates), nil
}

// importPortablePage persists one archive page using portable metadata instead of target defaults.
func (s *PortableImporter) importPortablePage(
	ctx context.Context,
	candidate PortableImportedPage,
	actor domain.User,
) error {
	slug := md.Slug(candidate.Slug)
	if slug == "" {
		return domain.NewValidationError("slug", fmt.Sprintf("Invalid imported page path %q.", candidate.Slug))
	}

	previousSlug := ""
	if _, err := s.repository.GetPage(ctx, slug); err == nil {
		previousSlug = slug
	} else if !errors.Is(err, domain.ErrNotFound) {
		return err
	}

	_, err := s.mutations.savePortable(ctx, s.repository, PageSaveInput{
		PreviousSlug:       previousSlug,
		Slug:               slug,
		Title:              candidate.Title,
		Icon:               candidate.Icon,
		Language:           candidate.Language,
		Markdown:           candidate.Markdown,
		Message:            "Imported from Kumbuka archive",
		Tags:               candidate.Tags,
		GroupIDs:           candidate.GroupIDs,
		Status:             candidate.Status,
		OwnerGroupID:       candidate.OwnerGroupID,
		ReviewIntervalDays: candidate.ReviewIntervalDays,
		DeprecatedTarget:   candidate.DeprecatedTarget,
		Properties:         candidate.Properties,
		Actor:              actor,
	})

	return err
}
