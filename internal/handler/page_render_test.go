package handler

import (
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageRenderArtifactRoundTrip(t *testing.T) {
	t.Parallel()
	rendered := md.RenderedPage{
		HTML:     "<h1 id=\"guide\">Guide</h1>",
		Contents: []md.Heading{{Level: 1, ID: "guide", Title: "Guide"}},
	}
	artifact, ok := pageRenderArtifact(rendered, "fingerprint")
	require.True(t, ok)
	assert.Equal(t, domain.PageRender{
		HTML:        rendered.HTML,
		Contents:    []domain.PageHeading{{Level: 1, ID: "guide", Title: "Guide"}},
		Fingerprint: "fingerprint",
	}, artifact)
	assert.Equal(t, rendered, renderedPageFromArtifact(artifact))
}

func TestPageRenderArtifactRejectsRequestLocalMetadata(t *testing.T) {
	t.Parallel()
	_, ok := pageRenderArtifact(md.RenderedPage{Inspectors: []plugin.Inspector{{ID: "variables"}}}, "fingerprint")
	assert.False(t, ok)
}
