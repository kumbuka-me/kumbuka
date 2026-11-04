package webview

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"testing/fstest"

	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRevisionFragmentIncludesStyledComparison verifies line markers and metadata remain template-owned.
func TestRevisionFragmentIncludesStyledComparison(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("../../web/src/templates/revisions.gohtml")
	require.NoError(t, err)

	appFS := testViewFS()
	appFS["templates/revisions.gohtml"] = &fstest.MapFile{Data: source}
	views, err := New(appFS, testViewsLogger(), "dev", "none", nil, RuntimeInfo{})
	require.NoError(t, err)

	records := revision.AnalyzeAll([]revision.Revision{{
		Number: 2, Author: "Alice", Markdown: "# New title\nSecond line\n", PreviousMarkdown: "# Old title\nSecond line\n",
	}})
	response := httptest.NewRecorder()
	views.RenderFragment(response, "page", "revision-list", RevisionsView{Revisions: records, RevisionSlug: "guide"})

	require.Equal(t, http.StatusOK, response.Code)
	html := response.Body.String()
	assert.Contains(t, html, "revision-patch-added")
	assert.Contains(t, html, "revision-patch-removed")
	assert.Contains(t, html, "revision-patch-line-number")
	assert.Contains(t, html, "Markdown changes")
	assert.NotContains(t, html, "--- revision 1")
	assert.NotContains(t, html, "+++ revision 2")
}
