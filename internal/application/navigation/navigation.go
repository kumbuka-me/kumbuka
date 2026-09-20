package navigation

import (
	"context"
	"maps"
	"strings"
	"sync"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
)

// navigationRepository contains navigation tree and icon operations.
type navigationRepository interface {
	NavigationPages(context.Context) ([]domain.Page, error)
	NavigationItems(context.Context) ([]domain.NavigationItem, error)
	NavigationIcons(context.Context) (map[string]string, error)
	SetNavigationIcon(context.Context, string, string) error
}

// pageFilter removes pages the actor may not view.
type pageFilter interface {
	FilterPages(context.Context, domain.User, []domain.Page) ([]domain.Page, error)
}

// Navigation exposes navigation tree and icon use cases.
type Navigation struct {
	// repository provides the persistence operations required by navigation.
	repository navigationRepository
	// access filters navigation collections for the current actor.
	access pageFilter
	// iconCatalog stores the icon catalog value used by navigation.
	iconCatalog *icons.Catalog

	// iconsMu stores the icons mu value used by navigation.
	iconsMu sync.RWMutex
	// icons maps keys to icons values used by navigation.
	icons map[string]string
	// iconsLoaded reports whether icons loaded applies to navigation.
	iconsLoaded bool
}

// NewNavigation constructs the navigation service.
func NewNavigation(repository navigationRepository, access pageFilter) *Navigation {
	return &Navigation{repository: repository, access: access, iconCatalog: icons.Builtin()}
}

// WithIconCatalog uses the active plugin-aware icon catalog for validation.
func (s *Navigation) WithIconCatalog(catalog *icons.Catalog) *Navigation {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	s.iconCatalog = catalog
	return s
}

// NavigationPages returns the page projection needed to build navigation.
func (s *Navigation) NavigationPages(ctx context.Context) ([]domain.Page, error) {
	return s.repository.NavigationPages(ctx)
}

// VisiblePages returns only navigation pages visible to the actor.
func (s *Navigation) VisiblePages(ctx context.Context, actor domain.User) ([]domain.Page, error) {
	pages, err := s.NavigationPages(ctx)
	if err != nil {
		return nil, err
	}
	return s.access.FilterPages(ctx, actor, pages)
}

// NavigationItems returns configured navigation folders and metadata.
func (s *Navigation) NavigationItems(ctx context.Context) ([]domain.NavigationItem, error) {
	return s.repository.NavigationItems(ctx)
}

// NavigationIcons returns configured icons keyed by navigation path. The icon
// set changes only through the navigation administration workflow, so cache it
// after the first load instead of querying the database for every page view.
func (s *Navigation) NavigationIcons(ctx context.Context) (map[string]string, error) {
	s.iconsMu.RLock()
	if s.iconsLoaded {
		icons := maps.Clone(s.icons)
		s.iconsMu.RUnlock()
		return icons, nil
	}
	s.iconsMu.RUnlock()

	// Serialize the first load so concurrent page requests do not all issue the
	// same database query while the cache is still cold.
	s.iconsMu.Lock()
	defer s.iconsMu.Unlock()

	if s.iconsLoaded {
		return maps.Clone(s.icons), nil
	}

	icons, err := s.repository.NavigationIcons(ctx)
	if err != nil {
		return nil, err
	}

	s.icons = maps.Clone(icons)
	if s.icons == nil {
		s.icons = make(map[string]string)
	}
	s.iconsLoaded = true
	return maps.Clone(s.icons), nil
}

// SetNavigationIcon sets or clears the icon for a navigation path.
func (s *Navigation) SetNavigationIcon(ctx context.Context, path, icon string) error {
	icon = strings.TrimSpace(icon)
	if !s.iconCatalog.IsIcon(icon) {
		return domain.NewValidationError("icon", "Choose an icon from the available icon catalog.")
	}
	if err := s.repository.SetNavigationIcon(ctx, path, icon); err != nil {
		return err
	}

	// Keep an already-loaded cache coherent with successful admin writes. A
	// cold cache remains cold and will load the complete set on first use.
	cachePath := strings.Trim(strings.TrimSpace(path), "/")
	s.iconsMu.Lock()
	if s.iconsLoaded {
		if icon == "" {
			delete(s.icons, cachePath)
		} else {
			s.icons[cachePath] = icon
		}
	}
	s.iconsMu.Unlock()

	return nil
}
