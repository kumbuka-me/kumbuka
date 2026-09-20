package pages

import (
	"context"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// renderArtifactRepository stores reusable page render artifacts.
type renderArtifactRepository interface {
	SavePageRender(context.Context, int64, time.Time, domain.PageRender) error
}

// RenderArtifacts owns persistence of reusable page render results.
type RenderArtifacts struct {
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
