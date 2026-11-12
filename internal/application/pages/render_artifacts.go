package pages

import (
	"context"
	"errors"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// renderArtifactRepository stores reusable page render artifacts.
type renderArtifactRepository interface {
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
}

// pageSVGRepository stores passive artifacts within the authorized page revision.
type pageSVGRepository interface {
	PutPageSVG(context.Context, int64, time.Time, []byte) (string, error)
	GetPageSVG(context.Context, int64, string) ([]byte, bool, error)
}

// RenderArtifacts owns persistence of reusable page render results.
type RenderArtifacts struct {
	// repository persists reusable render results for unchanged pages.
	repository renderArtifactRepository
}

// NewRenderArtifacts constructs render-artifact persistence.
func NewRenderArtifacts(repository renderArtifactRepository) *RenderArtifacts {
	return &RenderArtifacts{repository: repository}
}

// SavePageRender refreshes a reusable render artifact for an unchanged page.
func (c *RenderArtifacts) SavePageRender(ctx context.Context, pageID int64, updatedAt time.Time, render domain.PageRender) error {
	return c.repository.SavePageRender(ctx, pageID, updatedAt, render)
}

// PutPageSVG writes a passive SVG for the current page revision.
func (c *RenderArtifacts) PutPageSVG(ctx context.Context, id int64, revision time.Time, data []byte) (string, error) {
	store, ok := c.repository.(pageSVGRepository)
	if !ok {
		return "", errors.New("page SVG persistence unavailable")
	}
	return store.PutPageSVG(ctx, id, revision, data)
}

// GetPageSVG returns only an artifact still belonging to the current page revision.
func (c *RenderArtifacts) GetPageSVG(ctx context.Context, id int64, digest string) ([]byte, bool, error) {
	store, ok := c.repository.(pageSVGRepository)
	if !ok {
		return nil, false, errors.New("page SVG persistence unavailable")
	}
	return store.GetPageSVG(ctx, id, digest)
}
