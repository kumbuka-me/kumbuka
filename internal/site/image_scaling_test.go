package site

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticHTMLPreservesImageWidthsWhenRewritingURLs(t *testing.T) {
	t.Parallel()

	t.Run("640px", func(t *testing.T) {
		t.Parallel()
		rendered, err := testMarkdownRenderer(t).Render("![Diagram](images/diagram.png){width=640px}")
		require.NoError(t, err)

		got, searchText, err := processRenderedHTML(rendered, "guide/page.md", false, nil, "/kumbuka/")

		require.NoError(t, err)
		assert.Contains(t, got, `src="/kumbuka/guide/images/diagram.png"`)
		assert.Contains(t, strings.ReplaceAll(got, " ", ""), "style=\"width:640px\"")
		assert.NotContains(t, got, "{width=")
		assert.NotContains(t, searchText, "{width=")
	})

	t.Run("50%", func(t *testing.T) {
		t.Parallel()
		rendered, err := testMarkdownRenderer(t).Render("![Diagram](images/diagram.png){width=50%}")
		require.NoError(t, err)

		got, searchText, err := processRenderedHTML(rendered, "guide/page.md", false, nil, "/kumbuka/")

		require.NoError(t, err)
		assert.Contains(t, got, `src="/kumbuka/guide/images/diagram.png"`)
		assert.Contains(t, strings.ReplaceAll(got, " ", ""), "style=\"width:50%\"")
		assert.NotContains(t, got, "{width=")
		assert.NotContains(t, searchText, "{width=")
	})
}
