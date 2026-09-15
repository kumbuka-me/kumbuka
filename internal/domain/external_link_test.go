package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExternalLinkHoverTitle(t *testing.T) {
	t.Parallel()

	t.Run("uses existing label and description by default", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "Repository — GitHub", ExternalLinkHoverTitle(ExternalLink{Label: "Repository", Description: "GitHub"}))
	})

	t.Run("expands configurable placeholders with whitespace", func(t *testing.T) {
		t.Parallel()

		link := ExternalLink{Label: "Repository", Description: "GitHub", HoverText: "{{label }} | {{ description}}"}
		assert.Equal(t, "Repository | GitHub", ExternalLinkHoverTitle(link))
	})

	t.Run("preserves unknown placeholders", func(t *testing.T) {
		t.Parallel()

		link := ExternalLink{Label: "Repository", HoverText: "{{unknown}} {{label}}"}
		assert.Equal(t, "{{unknown}} Repository", ExternalLinkHoverTitle(link))
	})
}
