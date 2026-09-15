package handler

import (
	"github.com/kumbuka-me/kumbuka/internal/domain"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
)

func renderedPageFromArtifact(render domain.PageRender) md.RenderedPage {
	contents := make([]md.Heading, len(render.Contents))
	for index, heading := range render.Contents {
		contents[index] = md.Heading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}
	return md.RenderedPage{HTML: render.HTML, Contents: contents}
}

func pageRenderArtifact(rendered md.RenderedPage, fingerprint string) (domain.PageRender, bool) {
	if len(rendered.Inspectors) != 0 || len(rendered.ExportFields) != 0 {
		return domain.PageRender{}, false
	}
	contents := make([]domain.PageHeading, len(rendered.Contents))
	for index, heading := range rendered.Contents {
		contents[index] = domain.PageHeading{Level: heading.Level, ID: heading.ID, Title: heading.Title}
	}
	return domain.PageRender{HTML: rendered.HTML, Contents: contents, Fingerprint: fingerprint}, true
}
