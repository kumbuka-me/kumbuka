package service

import (
	"context"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/domain"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
)

const (
	// PageAccessView grants read access to a protected page path.
	PageAccessView = "view"
	// PageAccessEdit grants read and edit access to a protected page path.
	PageAccessEdit = "edit"
)

type accessRepository interface {
	PageAccess(context.Context, string, int64) (domain.PageAccess, error)
	PageAccessRules(context.Context) ([]domain.PageAccessRule, error)
	SavePageAccessRule(context.Context, string, int64, string) error
	DeletePageAccessRule(context.Context, int64) error
}

// Access owns inherited path authorization. Rules on the nearest matching path
// form an allow-list; paths without rules remain open to authenticated users.
type Access struct{ repository accessRepository }

// NewAccess constructs inherited page-path authorization use cases.
func NewAccess(repository accessRepository) *Access {
	return &Access{repository: repository}
}

// CanView reports whether a user may read the requested page path.
func (s *Access) CanView(ctx context.Context, user domain.User, path string) (bool, error) {
	if isAdministrator(user) {
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
	if isAdministrator(user) {
		return true, nil
	}
	if user.Role != "editor" {
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
	result := make([]domain.Page, 0, len(pages))

	for _, page := range pages {
		allowed, err := s.CanView(ctx, user, page.Slug)
		if err != nil {
			return nil, err
		}
		if !allowed {
			continue
		}

		result = append(result, page)
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
		return newValidationError("path", "A page path is required.")
	}
	if groupID <= 0 {
		return newValidationError("group_id", "Choose a group.")
	}
	if access != PageAccessView && access != PageAccessEdit {
		return newValidationError("access", "Choose view or edit access.")
	}
	return s.repository.SavePageAccessRule(ctx, path, groupID, access)
}

// DeletePageAccessRule removes one inherited path rule.
func (s *Access) DeletePageAccessRule(ctx context.Context, id int64) error {
	if id <= 0 {
		return newValidationError("rule", "Choose a valid access rule.")
	}
	return s.repository.DeletePageAccessRule(ctx, id)
}

// normalizeAccessPath canonicalizes a page path for authorization lookups.
func normalizeAccessPath(path string) string {
	return strings.Trim(md.Slug(path), "/")
}

// isAdministrator reports whether the user has effective administrator access.
func isAdministrator(user domain.User) bool {
	return user.Role == "admin" || user.ExternalAdmin
}
