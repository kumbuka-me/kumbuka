package plugins

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

// UpdateApproval binds administrator consent to the exact catalog version and newly requested permissions that were reviewed.
type UpdateApproval struct {
	// Version is the catalog release shown to the administrator.
	Version string
	// Permissions contains the exact newly requested permissions approved for that release.
	Permissions []string
}

// PermissionApprovalRequiredError reports additional capabilities introduced by a catalog update.
type PermissionApprovalRequiredError struct {
	// PluginID identifies the installed plugin requesting additional capabilities.
	PluginID string
	// Version is the catalog version that requests the additional capabilities.
	Version string
	// Permissions contains only permissions that are new relative to the installed version.
	Permissions []string
}

// Error describes the required explicit administrator approval without exposing package internals.
func (e *PermissionApprovalRequiredError) Error() string {
	return fmt.Sprintf("plugin %s update to %s requires approval for new permissions: %v", e.PluginID, e.Version, e.Permissions)
}

// UpdateFailure records one independent plugin update that could not be applied.
type UpdateFailure struct {
	// PluginID identifies the plugin whose update was not applied.
	PluginID string
	// Err is the update-specific failure. It may be a PermissionApprovalRequiredError.
	Err error
}

// UpdateAllResult reports successful and unsuccessful updates from one bulk operation.
type UpdateAllResult struct {
	// Updated contains plugin IDs whose catalog update was applied successfully.
	Updated []string
	// Failed contains independent plugin failures in deterministic plugin-ID order.
	Failed []UpdateFailure
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

// Status returns the latest catalog refresh state or an empty status when no catalog is configured.
func (a *Admin) Status() PluginUpdateStatus {
	if a.catalog == nil {
		return PluginUpdateStatus{}
	}
	return a.catalog.Status()
}

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

// Update downloads and applies one compatible installed-plugin update. Additional manifest permissions require an explicit administrator approval before the package is activated.
func (a *Admin) Update(ctx context.Context, id string, approvals ...UpdateApproval) error {
	if !a.CatalogAvailable() {
		return errors.New("plugin update catalog is unavailable")
	}
	installed, ok := installedPlugin(a.manager.Plugins(), id)
	if !ok {
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
	return a.updateRelease(ctx, installed, release, approvals)
}

// UpdateAll applies every independent compatible update in sorted order. One download, permission approval, validation, or activation failure never prevents later plugins from being attempted.
func (a *Admin) UpdateAll(ctx context.Context) (UpdateAllResult, error) {
	updates, err := a.Available()
	if err != nil {
		return UpdateAllResult{}, fmt.Errorf("check plugin update catalog: %w", err)
	}

	installed := a.manager.Plugins()
	result := UpdateAllResult{}
	for _, id := range installedUpdateIDs(installed, updates) {
		item, ok := installedPlugin(installed, id)
		if !ok {
			continue
		}
		if err := a.updateRelease(ctx, item, updates[id], nil); err != nil {
			result.Failed = append(result.Failed, UpdateFailure{PluginID: id, Err: err})
			continue
		}
		result.Updated = append(result.Updated, id)
	}
	return result, nil
}

// updateRelease downloads, validates, authorizes, and activates one known catalog release.
func (a *Admin) updateRelease(ctx context.Context, installed plugin.LoadedPlugin, release domain.PluginRelease, approvals []UpdateApproval) error {
	id := installed.Manifest.ID
	archive, err := a.catalog.Download(ctx, id, release.Version)
	if err != nil {
		return fmt.Errorf("download plugin update: %w", err)
	}
	packageData, err := validateUpdatePackage(id, archive)
	if err != nil {
		return err
	}
	if permissions := addedPermissions(installed.Manifest.Permissions, packageData.Manifest().Permissions); len(permissions) > 0 {
		if !permissionApprovalMatches(approvals, release.Version, permissions) {
			return &PermissionApprovalRequiredError{PluginID: id, Version: release.Version, Permissions: permissions}
		}
	}
	_, err = a.manager.Upgrade(ctx, id, archive)
	return err
}

// validateUpdatePackage validates downloaded bytes and enforces the catalog plugin identity before permission comparison or activation.
func validateUpdatePackage(id string, archive []byte) (*pluginpackage.Package, error) {
	packageData, err := pluginpackage.Read(archive)
	if err != nil {
		return nil, fmt.Errorf("validate %s update: %w", id, err)
	}
	if packageData.Manifest().ID != id {
		return nil, fmt.Errorf("validate %s update: invalid package identity", id)
	}
	return packageData, nil
}

// permissionApprovalMatches reports whether consent exactly matches the release and permission delta currently being activated.
func permissionApprovalMatches(approvals []UpdateApproval, version string, permissions []string) bool {
	if len(approvals) == 0 || approvals[0].Version != version {
		return false
	}
	approved := normalizedPermissions(approvals[0].Permissions)
	return slices.Equal(approved, normalizedPermissions(permissions))
}

// normalizedPermissions returns a sorted, de-duplicated permission set for exact approval comparison.
func normalizedPermissions(permissions []string) []string {
	result := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		if permission != "" && !slices.Contains(result, permission) {
			result = append(result, permission)
		}
	}
	sort.Strings(result)
	return result
}

// addedPermissions returns sorted target permissions absent from the currently installed manifest.
func addedPermissions(current, target []string) []string {
	permissions := make([]string, 0, len(target))
	for _, permission := range target {
		if !slices.Contains(current, permission) && !slices.Contains(permissions, permission) {
			permissions = append(permissions, permission)
		}
	}
	sort.Strings(permissions)
	return permissions
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

// installedPlugin returns one installed plugin by ID.
func installedPlugin(items []plugin.LoadedPlugin, id string) (plugin.LoadedPlugin, bool) {
	for _, item := range items {
		if item.Manifest.ID == id {
			return item, true
		}
	}
	return plugin.LoadedPlugin{}, false
}

// pluginInstalled reports whether a plugin ID occurs in the current manager inventory.
func pluginInstalled(items []plugin.LoadedPlugin, id string) bool {
	_, ok := installedPlugin(items, id)
	return ok
}
