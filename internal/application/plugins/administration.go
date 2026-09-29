package plugins

import (
	"context"
	"crypto/sha256"
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

// PermissionApproval binds administrator consent to the exact plugin package and complete requested permission set that were reviewed.
type PermissionApproval struct {
	// Operation is install, update, or upgrade and prevents consent from being reused across lifecycle actions.
	Operation string
	// PluginID identifies the package whose permissions were reviewed.
	PluginID string
	// Version is the package version shown to the administrator.
	Version string
	// Digest is the SHA-256 digest of the exact package bytes that were reviewed.
	Digest string
	// Permissions contains the complete permission set approved for that package.
	Permissions []string
}

// UpdateApproval is retained as the update-specific spelling of PermissionApproval.
type UpdateApproval = PermissionApproval

// PermissionApprovalRequiredError reports a package that must be reviewed before installation or activation.
type PermissionApprovalRequiredError struct {
	// Operation is install, update, or upgrade.
	Operation string
	// PluginID identifies the plugin requesting permission approval.
	PluginID string
	// Name is the human-readable plugin name from the package manifest.
	Name string
	// Provider is the package provider shown in administration.
	Provider string
	// Description is the package description shown in administration.
	Description string
	// Version is the package version that requires approval.
	Version string
	// Digest binds the approval to the exact package bytes.
	Digest string
	// Permissions contains the complete target permission set.
	Permissions []string
	// AddedPermissions contains target permissions absent from the installed version.
	AddedPermissions []string
	// RemovedPermissions contains installed permissions absent from the target version.
	RemovedPermissions []string
}

// Error describes the required explicit administrator approval without exposing package internals.
func (e *PermissionApprovalRequiredError) Error() string {
	if e.Operation == "install" {
		return fmt.Sprintf("plugin %s installation requires permission approval", e.PluginID)
	}
	return fmt.Sprintf("plugin %s %s to %s changes permissions and requires approval", e.PluginID, e.Operation, e.Version)
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

// Install validates one plugin package and requires explicit administrator review before any package is installed, including packages that request no permissions.
func (a *Admin) Install(ctx context.Context, archive []byte, approvals ...PermissionApproval) (plugin.LoadedPlugin, error) {
	packageData, err := pluginpackage.Read(archive)
	if err != nil {
		return plugin.LoadedPlugin{}, err
	}
	manifest := packageData.Manifest()
	if !permissionApprovalMatches(approvals, "install", manifest.ID, manifest.Version, manifest.Permissions, archive) {
		return plugin.LoadedPlugin{}, permissionApprovalRequired("install", pluginpackage.Manifest{}, manifest, manifest.Version, archive)
	}
	return a.manager.Install(ctx, archive)
}

// Upgrade validates package identity and requires explicit administrator approval whenever the complete permission set changes.
func (a *Admin) Upgrade(ctx context.Context, id string, archive []byte, approvals ...PermissionApproval) error {
	installed, ok := installedPlugin(a.manager.Plugins(), id)
	if !ok {
		return errors.New("plugin is not installed")
	}
	packageData, err := pluginpackage.Read(archive)
	if err != nil {
		return err
	}
	manifest := packageData.Manifest()
	if manifest.ID != id {
		return errors.New("uploaded package must have the same plugin ID")
	}
	if permissionSetChanged(installed.Manifest.Permissions, manifest.Permissions) && !permissionApprovalMatches(approvals, "upgrade", manifest.ID, manifest.Version, manifest.Permissions, archive) {
		return permissionApprovalRequired("upgrade", installed.Manifest, manifest, manifest.Version, archive)
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

// Update downloads and applies one compatible installed-plugin update. Any permission-set change requires explicit administrator approval before the package is activated.
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

const pluginUpdateConcurrency = 4

// updateOutcome contains the terminal result for one independent catalog update.
type updateOutcome struct {
	pluginID string
	err      error
}

// UpdateAll applies every independent compatible update with bounded concurrency. Downloads and package validation may run four at a time, while the plugin manager keeps final lifecycle publication serialized. One download, permission approval, validation, or activation failure never prevents another plugin from being attempted.
func (a *Admin) UpdateAll(ctx context.Context) (UpdateAllResult, error) {
	updates, err := a.Available()
	if err != nil {
		return UpdateAllResult{}, fmt.Errorf("check plugin update catalog: %w", err)
	}

	installed := a.manager.Plugins()
	ids := installedUpdateIDs(installed, updates)
	if len(ids) == 0 {
		return UpdateAllResult{}, nil
	}

	jobs := make(chan string, len(ids))
	outcomes := make(chan updateOutcome, len(ids))
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)

	workers := min(pluginUpdateConcurrency, len(ids))
	for range workers {
		go func() {
			for id := range jobs {
				item, ok := installedPlugin(installed, id)
				if !ok {
					outcomes <- updateOutcome{pluginID: id, err: errors.New("plugin is not installed")}
					continue
				}

				outcomes <- updateOutcome{
					pluginID: id,
					err:      a.updateRelease(ctx, item, updates[id], nil),
				}
			}
		}()
	}

	result := UpdateAllResult{}
	for range ids {
		outcome := <-outcomes
		if outcome.err != nil {
			result.Failed = append(result.Failed, UpdateFailure{PluginID: outcome.pluginID, Err: outcome.err})
			continue
		}
		result.Updated = append(result.Updated, outcome.pluginID)
	}

	sort.Strings(result.Updated)
	sort.Slice(result.Failed, func(i, j int) bool { return result.Failed[i].PluginID < result.Failed[j].PluginID })
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
	manifest := packageData.Manifest()
	if permissionSetChanged(installed.Manifest.Permissions, manifest.Permissions) && !permissionApprovalMatches(approvals, "update", manifest.ID, release.Version, manifest.Permissions, archive) {
		return permissionApprovalRequired("update", installed.Manifest, manifest, release.Version, archive)
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

// permissionApprovalRequired constructs a review challenge bound to the exact package bytes and permission transition.
func permissionApprovalRequired(operation string, current, target pluginpackage.Manifest, version string, archive []byte) *PermissionApprovalRequiredError {
	added, removed := permissionChanges(current.Permissions, target.Permissions)
	return &PermissionApprovalRequiredError{
		Operation:          operation,
		PluginID:           target.ID,
		Name:               target.Name,
		Provider:           target.Provider,
		Description:        target.Description,
		Version:            version,
		Digest:             pluginArchiveDigest(archive),
		Permissions:        normalizedPermissions(target.Permissions),
		AddedPermissions:   added,
		RemovedPermissions: removed,
	}
}

// permissionApprovalMatches reports whether consent exactly matches the package identity, reviewed version, digest, and complete target permission set currently being activated.
func permissionApprovalMatches(approvals []PermissionApproval, operation, pluginID, version string, permissions []string, archive []byte) bool {
	digest := pluginArchiveDigest(archive)
	targetPermissions := normalizedPermissions(permissions)
	for _, approved := range approvals {
		if approved.Operation == operation &&
			approved.PluginID == pluginID &&
			approved.Version == version &&
			approved.Digest == digest &&
			slices.Equal(normalizedPermissions(approved.Permissions), targetPermissions) {
			return true
		}
	}
	return false
}

// pluginArchiveDigest returns the canonical lower-case SHA-256 digest used to bind approval to package bytes.
func pluginArchiveDigest(archive []byte) string {
	digest := sha256.Sum256(archive)
	return fmt.Sprintf("%x", digest[:])
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

// permissionSetChanged reports whether two permission sets differ, ignoring ordering and duplicates.
func permissionSetChanged(current, target []string) bool {
	return !slices.Equal(normalizedPermissions(current), normalizedPermissions(target))
}

// permissionChanges returns the sorted additions and removals between two permission sets.
func permissionChanges(current, target []string) ([]string, []string) {
	current = normalizedPermissions(current)
	target = normalizedPermissions(target)
	added := make([]string, 0, len(target))
	removed := make([]string, 0, len(current))
	for _, permission := range target {
		if !slices.Contains(current, permission) {
			added = append(added, permission)
		}
	}
	for _, permission := range current {
		if !slices.Contains(target, permission) {
			removed = append(removed, permission)
		}
	}
	return added, removed
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
