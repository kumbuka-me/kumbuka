package pages

import (
	"context"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// pageAuthorization applies resource-level page access consistently across page use cases.
type pageAuthorization struct {
	// policy decides whether an actor may view or edit a page path.
	policy appaccess.Policy
}

// requireView enforces page visibility when a resource access policy is configured.
func (a pageAuthorization) requireView(ctx context.Context, actor domain.User, slug string) error {
	if a.policy == nil {
		return nil
	}
	return appaccess.RequireView(ctx, a.policy, actor, slug)
}

// requireEdit enforces page mutation access when a resource access policy is configured.
func (a pageAuthorization) requireEdit(ctx context.Context, actor domain.User, slug string) error {
	if a.policy == nil {
		return nil
	}
	return appaccess.RequireEdit(ctx, a.policy, actor, slug)
}
