package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// adminPageAccessServiceStub supplies access rules for the page-access screen.
type adminPageAccessServiceStub struct {
	pageAccessAdmin
	rules []domain.PageAccessRule
}

func (s adminPageAccessServiceStub) PageAccessRules(context.Context) ([]domain.PageAccessRule, error) {
	return s.rules, nil
}

// adminPageAccessGroupStub supplies collaboration groups for the page-access screen.
type adminPageAccessGroupStub struct {
	groupReader
	groups []domain.Group
}

func (s adminPageAccessGroupStub) Groups(context.Context) ([]domain.Group, error) {
	return s.groups, nil
}

// adminPageAccessNavigationStub supplies the administrator's complete page-path catalog.
type adminPageAccessNavigationStub struct {
	navigationService
	pages []domain.Page
}

func (s adminPageAccessNavigationStub) NavigationPages(context.Context) ([]domain.Page, error) {
	return s.pages, nil
}

func TestAdminPageAccessPopulatesSharedPathBreadcrumbCatalog(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{})
	browserContext := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: domain.UserRoleAdmin}}, nil
	}}
	navigation := adminPageAccessNavigationStub{pages: []domain.Page{
		{Slug: "platform", Title: "Platform"},
		{Slug: "platform/kubernetes", Title: "Kubernetes"},
	}}

	request := httptest.NewRequest(http.MethodGet, "/admin/permissions", nil)
	response := httptest.NewRecorder()
	AdminPageAccess(
		browserContext,
		adminPageAccessServiceStub{},
		adminPageAccessGroupStub{},
		navigation,
		views,
	).ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), `data-slug="platform"`)
	assert.Contains(t, response.Body.String(), `data-label="Platform"`)
	assert.Contains(t, response.Body.String(), `data-slug="platform/kubernetes"`)
	assert.Contains(t, response.Body.String(), `data-label="Platform / Kubernetes"`)
}
