package plugins

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Catalog supplies compatible releases and verified download bytes.
type Catalog interface {
	Refresh(context.Context) error
	Available() (map[string]domain.PluginRelease, error)
	Download(context.Context, string, string) ([]byte, error)
	Status() PluginUpdateStatus
}

// lifecycleManager applies plugin state changes and exposes the active inventory.
type lifecycleManager interface {
	Plugins() []plugin.LoadedPlugin
	IsRequired(string) bool
	Install(context.Context, []byte) (plugin.LoadedPlugin, error)
	Upgrade(context.Context, string, []byte) (plugin.LoadedPlugin, error)
	Enable(context.Context, string) error
	Disable(context.Context, string) error
	Uninstall(context.Context, string) error
}

// Admin coordinates plugin lifecycle changes and catalog upgrades.
type Admin struct {
	// manager applies plugin lifecycle operations.
	manager lifecycleManager
	// catalog finds and downloads compatible updates.
	catalog Catalog
}

// NewAdmin constructs lifecycle administration around the plugin manager and optional catalog.
func NewAdmin(manager lifecycleManager, catalog Catalog) *Admin {
	if manager == nil {
		return nil
	}
	return &Admin{manager: manager, catalog: catalog}
}

// Plugins returns the current plugin inventory.
func (a *Admin) Plugins() []plugin.LoadedPlugin { return a.manager.Plugins() }

// IsRequired reports whether the plugin is a protected system dependency.
func (a *Admin) IsRequired(id string) bool { return a.manager.IsRequired(id) }

// HasPlugin reports whether the plugin exists in the active inventory.
func (a *Admin) HasPlugin(id string) bool { return pluginInstalled(a.manager.Plugins(), id) }

// CatalogAvailable reports whether update checks were configured.
func (a *Admin) CatalogAvailable() bool { return a.catalog != nil }

// Refresh checks the configured first-party catalog.
func (a *Admin) Refresh(ctx context.Context) error {
	if a.catalog == nil {
		return errors.New("plugin update catalog is unavailable")
	}
	return a.catalog.Refresh(ctx)
}

// Status returns the latest catalog refresh state.
func (a *Admin) Status() PluginUpdateStatus { return a.catalog.Status() }

// Available returns compatible updates when catalog access succeeds.
func (a *Admin) Available() (map[string]domain.PluginRelease, error) {
	if a.catalog == nil {
		return nil, errors.New("plugin update catalog is unavailable")
	}
	return a.catalog.Available()
}

// Install validates and installs one plugin package.
func (a *Admin) Install(ctx context.Context, archive []byte) (plugin.LoadedPlugin, error) {
	if _, err := pluginpackage.Read(archive); err != nil {
		return plugin.LoadedPlugin{}, err
	}
	return a.manager.Install(ctx, archive)
}

// Upgrade validates package identity before replacing the installed plugin.
func (a *Admin) Upgrade(ctx context.Context, id string, archive []byte) error {
	packageData, err := pluginpackage.Read(archive)
	if err != nil {
		return err
	}
	if packageData.Manifest().ID != id {
		return errors.New("uploaded package must have the same plugin ID")
	}
	_, err = a.manager.Upgrade(ctx, id, archive)
	return err
}

// Enable activates an installed plugin.
func (a *Admin) Enable(ctx context.Context, id string) error { return a.manager.Enable(ctx, id) }

// Disable deactivates an installed plugin.
func (a *Admin) Disable(ctx context.Context, id string) error { return a.manager.Disable(ctx, id) }

// Uninstall removes an installed plugin.
func (a *Admin) Uninstall(ctx context.Context, id string) error { return a.manager.Uninstall(ctx, id) }

// Update downloads and applies one compatible installed-plugin update.
func (a *Admin) Update(ctx context.Context, id string) error {
	if !a.CatalogAvailable() {
		return errors.New("plugin update catalog is unavailable")
	}
	if !pluginInstalled(a.manager.Plugins(), id) {
		return errors.New("plugin is not installed")
	}
	updates, err := a.catalog.Available()
	if err != nil {
		return fmt.Errorf("check plugin update catalog: %w", err)
	}
	release, ok := updates[id]
	if !ok {
		return errors.New("no newer compatible plugin release is available")
	}
	archive, err := a.catalog.Download(ctx, id, release.Version)
	if err != nil {
		return fmt.Errorf("download plugin update: %w", err)
	}
	return a.Upgrade(ctx, id, archive)
}

// UpdateAll validates every download before upgrading in sorted order and reports completed IDs.
func (a *Admin) UpdateAll(ctx context.Context) ([]string, error) {
	updates, err := a.Available()
	if err != nil {
		return nil, fmt.Errorf("check plugin update catalog: %w", err)
	}
	ids := installedUpdateIDs(a.manager.Plugins(), updates)
	pending := make([]pendingUpdate, 0, len(ids))
	for _, id := range ids {
		archive, err := a.catalog.Download(ctx, id, updates[id].Version)
		if err != nil {
			return nil, fmt.Errorf("download %s update: %w", id, err)
		}
		packageData, err := pluginpackage.Read(archive)
		if err != nil || packageData.Manifest().ID != id {
			return nil, fmt.Errorf("validate %s update: invalid package identity or contents", id)
		}
		pending = append(pending, pendingUpdate{id: id, archive: archive})
	}
	updated := make([]string, 0, len(pending))
	for _, item := range pending {
		if err := a.Upgrade(ctx, item.id, item.archive); err != nil {
			return updated, fmt.Errorf("upgrade %s: %w", item.id, err)
		}
		updated = append(updated, item.id)
	}
	return updated, nil
}

// pendingUpdate stores validated package bytes before an upgrade begins.
type pendingUpdate struct {
	// id is the installed plugin identifier.
	id string
	// archive is the downloaded package payload.
	archive []byte
}

// installedUpdateIDs returns sorted IDs available for currently installed plugins.
func installedUpdateIDs(installed []plugin.LoadedPlugin, updates map[string]domain.PluginRelease) []string {
	ids := make([]string, 0, len(updates))
	for id := range updates {
		if pluginInstalled(installed, id) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// pluginInstalled reports whether a plugin ID occurs in the current manager inventory.
func pluginInstalled(items []plugin.LoadedPlugin, id string) bool {
	for _, item := range items {
		if item.Manifest.ID == id {
			return true
		}
	}
	return false
}
