package icons

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testResourceProvider struct {
	version   string
	resources []Resource
	err       error
}

func (p *testResourceProvider) IconResourceVersion() string { return p.version }
func (p *testResourceProvider) IconResources() ([]Resource, error) {
	return p.resources, p.err
}

func testPluginCatalog() *Catalog {
	return NewCatalog(&testResourceProvider{
		version: "simple-icons-v1",
		resources: []Resource{{
			Source: "Simple Icons",
			Data: []byte(`{
  "format": 1,
  "icons": [
    {"name":"github-simple","label":"GitHub","view_box":"0 0 24 24","paths":["M12 0C5.37 0 0 5.37 0 12z"]},
    {"name":"gitlab-simple","label":"GitLab","view_box":"0 0 24 24","paths":["M1 2L12 23L23 2z"]}
  ]
}`),
		}},
	})
}

func TestBuiltinOptionsAreUnique(t *testing.T) {
	t.Parallel()

	seen := make(map[string]bool)
	for _, option := range Options() {
		assert.NotEmpty(t, option.Name)
		assert.Falsef(t, seen[option.Name], "duplicate icon %q", option.Name)
		assert.Truef(t, IsIcon(option.Name), "icon %q is not accepted", option.Name)
		seen[option.Name] = true
	}
}

func TestBuiltinCatalogContainsOnlyCoreIcons(t *testing.T) {
	t.Parallel()

	assert.True(t, IsIcon(""))
	assert.True(t, IsIcon("search-lucide"))
	assert.False(t, IsIcon("github-simple"))
	assert.False(t, IsIcon("search"))
	assert.False(t, IsIcon("not-a-real-icon-lucide"))
}

func TestPluginIconResourceJoinsCatalog(t *testing.T) {
	t.Parallel()

	catalog := testPluginCatalog()
	assert.True(t, catalog.IsIcon("search-lucide"))
	assert.True(t, catalog.IsIcon("github-simple"))

	option := findOption(catalog.Search("simple icons", 20), "github-simple")
	require.NotNil(t, option)
	assert.Equal(t, "GitHub", option.Label)
	assert.Equal(t, "Simple Icons", option.Source)

	svg := string(catalog.SVG("github-simple", 18))
	assert.Contains(t, svg, `class="plugin-icon"`)
	assert.Contains(t, svg, `width="18"`)
	assert.Contains(t, svg, `height="18"`)
	assert.Contains(t, svg, `fill="currentColor"`)
	assert.Contains(t, svg, `<path d="M12 0C5.37 0 0 5.37 0 12z"></path>`)
}

func TestPluginIconResourceRefreshesWithProviderVersion(t *testing.T) {
	t.Parallel()

	provider := &testResourceProvider{version: "one", resources: testPluginCatalog().provider.(*testResourceProvider).resources}
	catalog := NewCatalog(provider)
	require.True(t, catalog.IsIcon("github-simple"))

	provider.version = "two"
	provider.resources = nil
	assert.False(t, catalog.IsIcon("github-simple"))
	assert.True(t, catalog.IsIcon("search-lucide"))
}

func TestInvalidPluginIconResourceFailsClosed(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog(&testResourceProvider{
		version:   "broken",
		resources: []Resource{{Source: "Bad", Data: []byte(`{"format":1,"icons":[{"name":"search-lucide","label":"Override","view_box":"0 0 24 24","paths":[]}]}`)}},
	})
	assert.True(t, catalog.IsIcon("search-lucide"))
	assert.Len(t, catalog.Search("Override", 10), 0)

	failed := NewCatalog(&testResourceProvider{version: "error", err: errors.New("boom")})
	assert.True(t, failed.IsIcon("search-lucide"))
}

func TestPluginIconCannotOverrideBuiltin(t *testing.T) {
	t.Parallel()

	catalog := NewCatalog(&testResourceProvider{
		version: "collision",
		resources: []Resource{{Source: "Override", Data: []byte(`{
  "format":1,
  "icons":[{"name":"search-lucide","label":"Override","view_box":"0 0 24 24","paths":["M0 0h1"]}]
}`)}},
	})

	option := findOption(catalog.Search("search-lucide", 10), "search-lucide")
	require.NotNil(t, option)
	assert.Equal(t, "Lucide", option.Source)
	assert.Contains(t, string(catalog.SVG("search-lucide", 18)), `class="lucide-icon"`)
}

func TestSearchCatalogPagination(t *testing.T) {
	t.Parallel()

	catalog := testPluginCatalog()
	first, hasMore := catalog.SearchPage("", 0, 3)
	require.Len(t, first, 3)
	assert.True(t, hasMore)

	second, _ := catalog.SearchPage("", len(first), 3)
	require.Len(t, second, 3)
	assert.NotEqual(t, first[len(first)-1].Name, second[0].Name)

	all := catalog.Options()
	last, hasMore := catalog.SearchPage("", len(all)-1, 3)
	assert.Len(t, last, 1)
	assert.False(t, hasMore)
}

func TestValidName(t *testing.T) {
	t.Parallel()

	assert.True(t, ValidName("github-simple"))
	assert.True(t, ValidName("brand.icon_1"))
	assert.False(t, ValidName(""))
	assert.False(t, ValidName("GitHub-simple"))
	assert.False(t, ValidName("icon space"))
}

func findOption(options []Option, name string) *Option {
	for index := range options {
		if options[index].Name == name {
			return &options[index]
		}
	}
	return nil
}
