package webview

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminHealthUsesCanonicalLinksAndScrollsAfterFifteenPages(t *testing.T) {
	t.Parallel()

	views, err := New(web.Assets, testViewsLogger(), "test", "test", nil, RuntimeInfo{RoutePrefix: "/kumbuka"})
	require.NoError(t, err)

	for _, count := range []int{15, 16} {
		t.Run(fmt.Sprintf("%d pages", count), func(t *testing.T) {
			t.Parallel()

			pages := make([]domain.Page, count)
			for i := range pages {
				pages[i] = domain.Page{
					ID:    int64(i + 7),
					Slug:  fmt.Sprintf("docs/page-%d", i),
					Title: fmt.Sprintf("Page %d", i),
				}
			}

			view := AdminHealthView{
				Layout: Layout{
					Title:        "Documentation health",
					RoutePrefix:  "/kumbuka",
					AdminSection: "health",
					User:         domain.User{ID: 1, Role: "admin"},
					Preferences:  domain.DefaultUserPreferences(),
				},
				DocumentationHealth: domain.DocumentationHealth{
					BrokenLinks:   []domain.BrokenWikiLink{{SourceSlug: "docs/source", SourceTitle: "Source", TargetSlug: "missing"}},
					OrphanPages:   pages,
					UntaggedPages: pages,
					UniconedPages: pages,
					StalePages:    pages,
					ReviewDue:     pages,
					DraftPages:    pages,
					Deprecated:    pages,
				},
			}

			response := httptest.NewRecorder()
			views.Render(response, "admin_health", view)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())
			body := response.Body.String()
			assert.Equal(t, 7, strings.Count(body, `href="/kumbuka/p/7/docs/page-0"`))
			assert.Equal(t, 7, strings.Count(body, fmt.Sprintf(`href="/kumbuka/p/%d/docs/page-%d"`, count+6, count-1)))
			assert.Contains(t, body, fmt.Sprintf(`<strong>%d</strong><span>Orphan pages</span>`, count))
			assert.Contains(t, body, `href="/kumbuka/edit/docs/source"`)
			assert.NotContains(t, body, "/p/0/")
			if count > 15 {
				assert.Contains(t, body, `aria-label="Orphan pages"`)
				assert.Contains(t, body, `tabindex="0"`)
			} else {
				assert.NotContains(t, body, `aria-label="Orphan pages"`)
			}
		})
	}
}
