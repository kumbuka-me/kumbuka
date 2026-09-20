package access

import (
	"context"
	"slices"
	"strings"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

const (
	// PageAccessView grants read access to a protected page path.
	PageAccessView = "view"
	// PageAccessEdit grants read and edit access to a protected page path.
	PageAccessEdit = "edit"
)

type accessRepository interface {
	PageAccess(context.Context, string, int64) (domain.PageAccess, error)
	PageAccessBatch(context.Context, []string, int64) (map[string]domain.PageAccess, error)
	PageAccessRules(context.Context) ([]domain.PageAccessRule, error)
	SavePageAccessRule(context.Context, string, int64, string) error
	DeletePageAccessRule(context.Context, int64) error
}

// Access owns inherited path authorization. Rules on the nearest matching path
// form an allow-list; paths without rules remain open to authenticated users.
type Access struct {
	// repository provides the persistence operations required by access.
	repository accessRepository
}

// NewAccess constructs inherited page-path authorization use cases.
func NewAccess(repository accessRepository) *Access {
	return &Access{repository: repository}
}

// CanView reports whether a user may read the requested page path.
func (s *Access) CanView(ctx context.Context, user domain.User, path string) (bool, error) {
	if user.IsAdministrator() {
		return true, nil
	}
	access, err := s.repository.PageAccess(ctx, normalizeAccessPath(path), user.ID)
	if err != nil {
		return false, err
	}
	return !access.Restricted || access.CanView, nil
}

// CanEdit reports whether a user may modify the requested page path.
func (s *Access) CanEdit(ctx context.Context, user domain.User, path string) (bool, error) {
	if user.IsAdministrator() {
		return true, nil
	}
	if !user.CanEditContent() {
		return false, nil
	}
	access, err := s.repository.PageAccess(ctx, normalizeAccessPath(path), user.ID)
	if err != nil {
		return false, err
	}
	return !access.Restricted || access.CanEdit, nil
}

// FilterPages removes pages the user may not view.
func (s *Access) FilterPages(ctx context.Context, user domain.User, pages []domain.Page) ([]domain.Page, error) {
	if user.IsAdministrator() || len(pages) == 0 {
		return slices.Clone(pages), nil
	}
	paths := make([]string, len(pages))
	for i, page := range pages {
		paths[i] = normalizeAccessPath(page.Slug)
	}
	grants, err := s.repository.PageAccessBatch(ctx, paths, user.ID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.Page, 0, len(pages))
	for i, page := range pages {
		grant, ok := grants[paths[i]]
		// An incomplete repository result must never expose a protected page.
		if ok && (!grant.Restricted || grant.CanView) {
			result = append(result, page)
		}
	}

	return result, nil
}

// PageAccessRules returns all configured inherited path rules.
func (s *Access) PageAccessRules(ctx context.Context) ([]domain.PageAccessRule, error) {
	return s.repository.PageAccessRules(ctx)
}

// SavePageAccessRule validates and persists one inherited path rule.
func (s *Access) SavePageAccessRule(ctx context.Context, path string, groupID int64, access string) error {
	path = normalizeAccessPath(path)
	if path == "" {
		return domain.NewValidationError("path", "A page path is required.")
	}
	if groupID <= 0 {
		return domain.NewValidationError("group_id", "Choose a group.")
	}
	if access != PageAccessView && access != PageAccessEdit {
		return domain.NewValidationError("access", "Choose view or edit access.")
	}
	return s.repository.SavePageAccessRule(ctx, path, groupID, access)
}

// DeletePageAccessRule removes one inherited path rule.
func (s *Access) DeletePageAccessRule(ctx context.Context, id int64) error {
	if id <= 0 {
		return domain.NewValidationError("rule", "Choose a valid access rule.")
	}
	return s.repository.DeletePageAccessRule(ctx, id)
}

// normalizeAccessPath canonicalizes a page path for authorization lookups.
func normalizeAccessPath(path string) string {
	return strings.Trim(md.Slug(path), "/")
}
