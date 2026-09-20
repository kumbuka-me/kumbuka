package pages

import (
	"context"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// historyRepository contains persisted revision-history reads.
type historyRepository interface {
	Revisions(context.Context, string) ([]revision.Revision, error)
}

// History owns actor-authorized revision history queries.
type History struct {
	repository historyRepository
	access     accessReader
}

// NewHistory constructs revision history queries.
func NewHistory(repository historyRepository, access accessReader) *History {
	return &History{repository: repository, access: access}
}

// RevisionsFor returns revision history only when the actor may view the page.
func (q *History) RevisionsFor(ctx context.Context, actor domain.User, slug string) ([]revision.Revision, error) {
	if err := appaccess.RequireView(ctx, q.access, actor, slug); err != nil {
		return nil, err
	}
	return q.repository.Revisions(ctx, slug)
}
