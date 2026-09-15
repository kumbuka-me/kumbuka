package navigation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFromPageSlugs(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "infrastructure/postgres/restore", Title: "Restore Postgres"},
		{Slug: "infrastructure/monitoring", Title: "Monitoring"},
		{Slug: "home", Title: "Home"},
	}, Options{ActiveSlug: "infrastructure/postgres/restore", ShowPageCounts: true, Icons: map[string]string{"infrastructure": "server"}})

	require.Len(t, tree, 2)
	assert.Equal(t, "infrastructure", tree[0].Title)
	assert.Len(t, tree[0].Children, 2)
	assert.True(t, tree[0].Root)
	assert.False(t, tree[0].Open)
	assert.True(t, tree[0].ContainsActive)
	assert.Equal(t, 2, tree[0].PageCount)
	assert.True(t, tree[0].ShowPageCount)
	assert.Equal(t, "server", tree[0].Icon)
	assert.Equal(t, "Home", tree[1].Title)
	assert.True(t, tree[1].Page)
	assert.Empty(t, tree[1].Icon)
}

func TestBuildDoesNotExpandFoldersForTheActivePage(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "applications", Title: "Applications"},
		{Slug: "applications/identity/keycloak", Title: "Keycloak"},
	}, Options{ActiveSlug: "applications"})

	require.Len(t, tree, 1)
	assert.True(t, tree[0].Active)
	assert.True(t, tree[0].ContainsActive)
	assert.False(t, tree[0].Open)
}

func TestBuildUsesPersistedExpandedFolders(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "applications/identity/keycloak", Title: "Keycloak"},
		{Slug: "applications/automation/jenkins", Title: "Jenkins"},
	}, Options{Expanded: []string{"applications", "applications/identity"}})

	require.Len(t, tree, 1)
	assert.True(t, tree[0].Open)

	identity := tree[0].Children[1]

	if identity.Slug != "applications/identity" {
		identity = tree[0].Children[0]
	}

	assert.Equal(t, "applications/identity", identity.Slug)
	assert.True(t, identity.Open)
}

func TestBuildUsesRawSlugSegmentsForFolders(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "api-guides/http_server/setup", Title: "Setup"},
	}, Options{})

	require.Len(t, tree, 1)
	assert.Equal(t, "api-guides", tree[0].Title)
	require.Len(t, tree[0].Children, 1)
	assert.Equal(t, "http_server", tree[0].Children[0].Title)
}

func TestBuildUsesIconsAtEveryNavigationDepth(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "personal", Title: "Personal", Icon: "user-lucide"},
		{Slug: "personal/household/laundry", Title: "Laundry", Icon: "washing-machine-lucide"},
	}, Options{Icons: map[string]string{
		"personal":           "user",
		"personal/household": "house",
	}})

	require.Len(t, tree, 1)
	assert.Equal(t, "user", tree[0].Icon)
	require.Len(t, tree[0].Children, 1)

	household := tree[0].Children[0]

	assert.Equal(t, "house", household.Icon)
	require.Len(t, household.Children, 1)
	assert.Equal(t, "washing-machine-lucide", household.Children[0].Icon)
}

func TestChildrenReturnsTheCompleteSubtree(t *testing.T) {
	t.Parallel()

	tree := Build([]Page{
		{Slug: "applications", Title: "Applications", Icon: "boxes-lucide"},
		{Slug: "applications/analytics/matomo", Title: "Matomo", Icon: "chart-no-axes-combined-lucide"},
		{Slug: "applications/analytics/matomo/maintenance", Title: "Matomo maintenance"},
		{Slug: "applications/automation/jenkins", Title: "Jenkins"},
	}, Options{})

	children := Children(tree, "applications")

	require.Len(t, children, 2)
	assert.Equal(t, "applications/analytics", children[0].Slug)
	require.Len(t, children[0].Children, 1)
	assert.Equal(t, "applications/analytics/matomo", children[0].Children[0].Slug)
	require.Len(t, children[0].Children[0].Children, 1)
	assert.Equal(t, "applications/analytics/matomo/maintenance", children[0].Children[0].Children[0].Slug)
}
