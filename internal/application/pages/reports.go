package pages

import (
	"context"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
)

// reportRepository contains the page reads exposed to reports and plugin capabilities.
type reportRepository interface {
	GetPage(context.Context, string) (domain.Page, error)
	Search(context.Context, string, int) ([]domain.Page, error)
	Backlinks(context.Context, string) ([]domain.Page, error)
	PageLinks(context.Context, string) ([]domain.PageLink, error)
	LatestRevision(context.Context, string) (revision.Revision, int, error)
	Revisions(context.Context, string) ([]revision.Revision, error)
}

// Reports supplies generic page data to reports and plugin capabilities.
type Reports struct {
	repository reportRepository
	access     accessReader
}

// NewReports constructs report and plugin page reads.
func NewReports(repository reportRepository, access accessReader) *Reports {
	return &Reports{repository: repository, access: access}
}

// Accessible binds report reads to one actor.
func (q *Reports) Accessible(actor domain.User) AccessibleCatalog {
	return NewAccessibleCatalog(q, q.access, actor)
}

// GetPageFor returns a page only when the actor may view it.
func (q *Reports) GetPageFor(ctx context.Context, actor domain.User, slug string) (domain.Page, error) {
	if err := appaccess.RequireView(ctx, q.access, actor, slug); err != nil {
		return domain.Page{}, err
	}
	return q.repository.GetPage(ctx, slug)
}

// GetPage returns one raw page for already-authorized report workflows.
func (q *Reports) GetPage(ctx context.Context, slug string) (domain.Page, error) {
	return q.repository.GetPage(ctx, slug)
}

// Search returns raw report search results; actor-scoped callers use Accessible.
func (q *Reports) Search(ctx context.Context, query string, limit int) ([]domain.Page, error) {
	return q.repository.Search(ctx, query, limit)
}

// Backlinks returns pages linking to slug.
func (q *Reports) Backlinks(ctx context.Context, slug string) ([]domain.Page, error) {
	return q.repository.Backlinks(ctx, slug)
}

// PageLinks returns link metadata for one page.
func (q *Reports) PageLinks(ctx context.Context, slug string) ([]domain.PageLink, error) {
	return q.repository.PageLinks(ctx, slug)
}

// LatestRevision returns a page's newest revision and total revision count.
func (q *Reports) LatestRevision(ctx context.Context, slug string) (revision.Revision, int, error) {
	return q.repository.LatestRevision(ctx, slug)
}

// Revisions returns a page's revision history.
func (q *Reports) Revisions(ctx context.Context, slug string) ([]revision.Revision, error) {
	return q.repository.Revisions(ctx, slug)
}
