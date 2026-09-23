package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// editorCatalogNavigationStub records actor-scoped page lookups from the editor catalog.
type editorCatalogNavigationStub struct {
	navigationService
	// actor is the user supplied to VisiblePages.
	actor domain.User
	// pages is the actor-visible page set returned to the endpoint.
	pages []domain.Page
	// visibleCalls counts actor-scoped page lookups.
	visibleCalls int
	// unfilteredCalls counts accidental unfiltered page lookups.
	unfilteredCalls int
}

// NavigationPages records an unexpected unfiltered catalog read.
func (s *editorCatalogNavigationStub) NavigationPages(context.Context) ([]domain.Page, error) {
	s.unfilteredCalls++
	return []domain.Page{{Slug: "private", Title: "Private"}}, nil
}

// VisiblePages returns the configured actor-visible catalog.
func (s *editorCatalogNavigationStub) VisiblePages(_ context.Context, actor domain.User) ([]domain.Page, error) {
	s.visibleCalls++
	s.actor = actor
	return s.pages, nil
}

// editorCatalogAliasStub returns no aliases for the focused page-visibility test.
type editorCatalogAliasStub struct{}

// PageAliasesFor returns an empty actor-visible alias map.
func (editorCatalogAliasStub) PageAliasesFor(context.Context, domain.User) (map[string]string, error) {
	return nil, nil
}

func TestEditorCatalogUsesActorVisiblePages(t *testing.T) {
	t.Parallel()

	actor := domain.User{ID: 42, Role: domain.UserRoleEditor, Enabled: true}
	navigation := &editorCatalogNavigationStub{pages: []domain.Page{{Slug: "public", Title: "Public"}}}
	request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/api/editor/catalog", nil), actor)
	response := httptest.NewRecorder()

	EditorCatalog(navigation, editorCatalogAliasStub{}, nil, nil, nil)(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, 1, navigation.visibleCalls)
	assert.Zero(t, navigation.unfilteredCalls)
	assert.Equal(t, actor, navigation.actor)

	var body struct {
		Pages []struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
		} `json:"pages"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	assert.Equal(t, "public", body.Pages[0].Slug)
	assert.Equal(t, "Public", body.Pages[0].Title)
}
