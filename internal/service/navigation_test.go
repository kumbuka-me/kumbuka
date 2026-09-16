package service

import (
	"context"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type navigationIconProvider struct{}

func (navigationIconProvider) IconResourceVersion() string { return "test" }

func (navigationIconProvider) IconResources() ([]icons.Resource, error) {
	return []icons.Resource{{
		Source: "Example",
		Data:   []byte(`{"format":1,"icons":[{"name":"brand-example","label":"Example","view_box":"0 0 24 24","paths":["M0 0H24V24H0z"]}]}`),
	}}, nil
}

type navigationRepositoryStub struct {
	icons     map[string]string
	iconCalls int
	setPath   string
	setIcon   string
	navigationRepository
}

func (s *navigationRepositoryStub) NavigationIcons(context.Context) (map[string]string, error) {
	s.iconCalls++
	return s.icons, nil
}

func (s *navigationRepositoryStub) SetNavigationIcon(_ context.Context, path, icon string) error {
	s.setPath = path
	s.setIcon = icon
	return nil
}

func TestNavigationIconsCachesRepositoryResult(t *testing.T) {
	t.Parallel()

	repository := &navigationRepositoryStub{icons: map[string]string{"platform": "folder-lucide"}}
	navigation := NewNavigation(repository)

	first, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)
	second, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, repository.iconCalls)
	assert.Equal(t, map[string]string{"platform": "folder-lucide"}, first)
	assert.Equal(t, first, second)

	// Callers must not be able to mutate the cached map.
	first["platform"] = "mutated"
	third, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "folder-lucide", third["platform"])
}

func TestSetNavigationIconUpdatesWarmCache(t *testing.T) {
	t.Parallel()

	repository := &navigationRepositoryStub{icons: map[string]string{"platform": "folder-lucide"}}
	navigation := NewNavigation(repository)

	_, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)

	icon := "book-lucide"
	err = navigation.SetNavigationIcon(context.Background(), "platform", icon)
	require.NoError(t, err)

	cached, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, repository.iconCalls)
	assert.Equal(t, "platform", repository.setPath)
	assert.Equal(t, icon, repository.setIcon)
	assert.Equal(t, icon, cached["platform"])
}

func TestSetNavigationIconAcceptsPluginResource(t *testing.T) {
	t.Parallel()

	repository := &navigationRepositoryStub{}
	navigation := NewNavigation(repository).WithIconCatalog(icons.NewCatalog(navigationIconProvider{}))

	err := navigation.SetNavigationIcon(context.Background(), "platform", "brand-example")

	require.NoError(t, err)
	assert.Equal(t, "brand-example", repository.setIcon)
}

var _ navigationRepository = (*navigationRepositoryStub)(nil)
