package navigation

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// navigationIconProvider supplies controlled navigation icon data for tests.
type navigationIconProvider struct{}

func (navigationIconProvider) IconResourceVersion() string { return "test" }

func (navigationIconProvider) IconResources() ([]icons.Resource, error) {
	return []icons.Resource{{
		Source: "Example",
		Data:   []byte(`{"format":1,"icons":[{"name":"brand-example","label":"Example","view_box":"0 0 24 24","paths":["M0 0H24V24H0z"]}]}`),
	}}, nil
}

// navigationPageRepositoryStub provides controllable navigation page repository behavior for tests.
type navigationPageRepositoryStub struct {
	// pages records the pages observed by the test double.
	pages []domain.Page
	// calls counts calls observed by the test double.
	calls int
	navigationRepository
}

func (s *navigationPageRepositoryStub) NavigationPages(context.Context) ([]domain.Page, error) {
	s.calls++
	return s.pages, nil
}

// navigationPageFilterStub provides controllable navigation page filter behavior for tests.
type navigationPageFilterStub struct {
	// calls counts calls observed by the test double.
	calls int
	// actor records the actor observed by the test double.
	actor domain.User
	// pages records the pages observed by the test double.
	pages []domain.Page
	// result configures the result returned by the test double.
	result []domain.Page
}

func (s *navigationPageFilterStub) FilterPages(_ context.Context, actor domain.User, pages []domain.Page) ([]domain.Page, error) {
	s.calls++
	s.actor = actor
	s.pages = append([]domain.Page(nil), pages...)
	return append([]domain.Page(nil), s.result...), nil
}

// navigationRepositoryStub provides controllable navigation repository behavior for tests.
type navigationRepositoryStub struct {
	// icons configures the icons used by the fixture.
	icons map[string]string
	// iconCalls counts icon calls observed by the test double.
	iconCalls int
	// setPath records the path passed to set operations.
	setPath string
	// setIcon records the icon passed to set operations.
	setIcon string
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
	navigation := NewNavigation(repository, nil).WithIconValidator(icons.Builtin())

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
	navigation := NewNavigation(repository, nil).WithIconValidator(icons.Builtin())

	_, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)

	icon := "book-lucide"
	err = navigation.SetNavigationIcon(context.Background(), " /platform/ ", icon)
	require.NoError(t, err)

	cached, err := navigation.NavigationIcons(context.Background())
	require.NoError(t, err)

	assert.Equal(t, 1, repository.iconCalls)
	assert.Equal(t, "platform", repository.setPath)
	assert.Equal(t, icon, repository.setIcon)
	assert.Equal(t, icon, cached["platform"])
}

func TestSetNavigationIconRejectsEmptyPathBeforePersistence(t *testing.T) {
	t.Parallel()

	repository := &navigationRepositoryStub{}
	err := NewNavigation(repository, nil).SetNavigationIcon(context.Background(), " / ", "")

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	require.Len(t, validation.Fields, 1)
	assert.Equal(t, "path", validation.Fields[0].Field)
	assert.Empty(t, repository.setPath)
}

func TestSetNavigationIconAcceptsPluginResource(t *testing.T) {
	t.Parallel()

	repository := &navigationRepositoryStub{}
	navigation := NewNavigation(repository, nil).WithIconValidator(icons.NewCatalog(navigationIconProvider{}))

	err := navigation.SetNavigationIcon(context.Background(), "platform", "brand-example")

	require.NoError(t, err)
	assert.Equal(t, "brand-example", repository.setIcon)
}

func TestVisiblePagesFiltersNavigationCollectionOnce(t *testing.T) {
	t.Parallel()

	pages := []domain.Page{{Slug: "open"}, {Slug: "private"}, {Slug: "docs/start"}}
	repository := &navigationPageRepositoryStub{pages: pages}
	filter := &navigationPageFilterStub{result: []domain.Page{pages[0], pages[2]}}
	actor := domain.User{ID: 42, Role: "viewer"}

	visible, err := NewNavigation(repository, filter).VisiblePages(context.Background(), actor)

	require.NoError(t, err)
	assert.Equal(t, []domain.Page{pages[0], pages[2]}, visible)
	assert.Equal(t, 1, repository.calls)
	assert.Equal(t, 1, filter.calls)
	assert.Equal(t, actor, filter.actor)
	assert.Equal(t, pages, filter.pages)
}

var _ navigationRepository = (*navigationRepositoryStub)(nil)
