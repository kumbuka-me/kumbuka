package access

import (
	"context"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"strings"
)

// Policy evaluates resource access independently of transport.
type Policy interface {
	CanView(context.Context, domain.User, string) (bool, error)
	CanEdit(context.Context, domain.User, string) (bool, error)
}

// RequireView hides unreadable resources using the shared domain not-found error.
func RequireView(ctx context.Context, policy Policy, actor domain.User, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	allowed, err := policy.CanView(ctx, actor, path)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrNotFound
	}
	return nil
}

// RequireEdit rejects writes to resources outside the actor's path grants.
func RequireEdit(ctx context.Context, policy Policy, actor domain.User, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	allowed, err := policy.CanEdit(ctx, actor, path)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	return nil
}
