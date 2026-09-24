package endpoint

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strings"

	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// validPluginUploadPart reports whether a multipart part is the required named plugin package file.
func validPluginUploadPart(formName, filename string) bool {
	return formName == "package" && filename != ""
}

// pluginUpdateService is the first-party catalog boundary used by plugin administration.
type pluginUpdateService interface {
	// Refresh checks the first-party catalog immediately.
	Refresh(context.Context) error
	// Available returns newer compatible releases for the currently loaded plugins.
	Available() (map[string]domain.PluginRelease, error)
	// Download retrieves and verifies one selected plugin release.
	Download(context.Context, string, string) ([]byte, error)
	// Status returns scheduled and manual catalog refresh state.
	Status() appplugins.PluginUpdateStatus
}

// AdminPlugins exposes package metadata and lifecycle operations through the existing administration layout. Routes apply browser authentication/admin authorization.
type AdminPlugins struct {
	// manager owns active plugin lifecycle state.
	manager *plugin.Manager
	// updates discovers and downloads compatible first-party plugin releases.
	updates pluginUpdateService
	// data loads shared administration view data.
	data browserContextLoader
	// views renders plugin administration responses.
	views *webview.Views
}

// NewAdminPlugins constructs the plugin administration handler.
func NewAdminPlugins(manager *plugin.Manager, updates pluginUpdateService, data browserContextLoader, views *webview.Views) *AdminPlugins {
	return &AdminPlugins{manager: manager, updates: updates, data: data, views: views}
}

// List renders the plugin inventory and optionally opens one plugin detail modal.
func (a *AdminPlugins) List(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, strings.TrimSpace(r.URL.Query().Get("plugin")), http.StatusOK, "")
}

// CheckUpdates refreshes the first-party plugin catalog immediately.
func (a *AdminPlugins) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	if a.updates == nil {
		a.render(w, r, "", http.StatusServiceUnavailable, "Plugin update checks are unavailable.")
		return
	}
	if err := a.updates.Refresh(r.Context()); err != nil {
		a.views.Logger().Warn("check plugin updates", "event", "plugin_catalog_manual_check_failed", "error", err, "actor_id", currentUser(r).ID)
		a.render(w, r, "", http.StatusBadGateway, "Could not check the plugin update catalog. The previous successful catalog remains available if one exists.")
		return
	}

	a.views.Logger().Info("plugin update catalog checked", "event", "plugin.catalog_check", "actor_id", currentUser(r).ID)
	http.Redirect(w, r, "/admin/plugins", http.StatusSeeOther)
}

// pendingPluginUpdate contains one downloaded and validated catalog package ready for upgrade.
type pendingPluginUpdate struct {
	// id identifies the installed plugin to upgrade.
	id string
	// archive contains the validated plugin package bytes.
	archive []byte
}

// render renders the plugin administration page.
func (a *AdminPlugins) render(w http.ResponseWriter, r *http.Request, id string, status int, message string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	layout, err := administrationData(r, a.data, a.views, "Plugins", "plugins")
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	data := webview.AdminPluginsView{Layout: layout, AdminPlugins: a.manager.Plugins()}
	a.populatePluginUpdateView(&data)
	if err := a.populatePluginDetails(&data, id); err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	if id != "" && !pluginInstalled(data.AdminPlugins, id) {
		http.NotFound(w, r)
		return
	}
	sort.Slice(data.AdminPlugins, func(i, j int) bool { return data.AdminPlugins[i].Manifest.Name < data.AdminPlugins[j].Manifest.Name })
	data.OpenPluginID = id
	data.PluginMessage = message
	a.views.RenderStatus(w, status, "admin_plugins", data)
}

// populatePluginUpdateView adds catalog status and available updates to the view model.
func (a *AdminPlugins) populatePluginUpdateView(data *webview.AdminPluginsView) {
	data.PluginUpdates = make(map[string]*webview.PluginUpdate)
	if a.updates == nil {
		data.PluginCatalogUnavailable = true
		return
	}
	status := a.updates.Status()
	data.PluginUpdateStatus = webview.PluginUpdateStatus{
		Available: true, Automatic: status.Automatic, LastAttempt: status.LastAttempt, LastSuccess: status.LastSuccess, LastError: status.LastError,
	}
	updates, err := a.updates.Available()
	if err != nil {
		data.PluginCatalogUnavailable = true
		return
	}
	for pluginID, release := range updates {
		data.PluginUpdates[pluginID] = &webview.PluginUpdate{Version: release.Version, ReleasedAt: release.ReleasedAt}
	}
}

// populatePluginDetails adds required-state, settings presence, and rendered README content.
func (a *AdminPlugins) populatePluginDetails(data *webview.AdminPluginsView, _ string) error {
	data.PluginRequiredIDs = make(map[string]bool, len(data.AdminPlugins))
	data.PluginHasSettings = make(map[string]bool, len(data.AdminPlugins))
	data.PluginREADMEs = make(map[string]template.HTML, len(data.AdminPlugins))
	for _, item := range data.AdminPlugins {
		pluginID := item.Manifest.ID
		data.PluginRequiredIDs[pluginID] = a.manager.IsRequired(pluginID)
		data.PluginHasSettings[pluginID] = manifestHasPluginSettings(item.Manifest)
		readme, err := renderPluginREADME(item.README)
		if err != nil {
			return err
		}
		data.PluginREADMEs[pluginID] = readme
	}
	return nil
}

// manifestHasPluginSettings reports whether a manifest exposes settings or administration resources.
func manifestHasPluginSettings(manifest pluginpackage.Manifest) bool {
	for _, module := range manifest.Modules {
		if module.Type == "settings" || module.Type == "admin-resource" {
			return true
		}
	}
	return false
}

// Install installs a plugin package from an administration request.
func (a *AdminPlugins) Install(w http.ResponseWriter, r *http.Request) {
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	archive, status, err := readPluginUpload(w, r)
	if err != nil {
		a.render(w, r, "", status, "Upload failed: "+err.Error()+".")
		return
	}
	if _, err = pluginpackage.Read(archive); err != nil {
		a.render(w, r, "", http.StatusUnprocessableEntity, "Invalid plugin package: "+err.Error())
		return
	}
	item, err := a.manager.Install(r.Context(), archive)
	if err != nil {
		a.failure(w, r, "", "install", err)
		return
	}
	a.audit(r, "install", item.Manifest.ID)
	http.Redirect(w, r, "/admin/plugins?plugin="+item.Manifest.ID, http.StatusSeeOther)
}

// Action applies a plugin lifecycle action from an administration request.
func (a *AdminPlugins) Action(w http.ResponseWriter, r *http.Request) {
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	id, action := r.PathValue("pluginID"), r.PathValue("action")
	if id == "all" && action == "update" {
		a.handleBulkUpdate(w, r)
		return
	}
	if err := a.runPluginAction(w, r, id, action); err != nil {
		if errors.Is(err, errPluginActionHandled) {
			return
		}
		a.failure(w, r, id, action, err)
		return
	}

	a.audit(r, action, id)
	http.Redirect(w, r, pluginActionDestination(r, id, action), http.StatusSeeOther)
}

var errPluginActionHandled = errors.New("plugin action response already written")

// handleBulkUpdate applies all catalog updates and renders the partial-success failure contract.
func (a *AdminPlugins) handleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	updated, err := a.updateAllFromCatalog(r.Context())
	for _, pluginID := range updated {
		a.audit(r, "update", pluginID)
	}
	if err != nil {
		a.views.Logger().Error("bulk plugin update failed", "event", "plugin.update_all_failed", "error", err, "actor_id", currentUser(r).ID)
		a.render(w, r, "", http.StatusUnprocessableEntity, "Could not update all plugins from the Kumbuka catalog. Plugins updated before the failure remain on their new versions; the remaining plugins were left unchanged.")
		return
	}
	http.Redirect(w, r, "/admin/plugins", http.StatusSeeOther)
}

// runPluginAction executes one lifecycle action and reports responses written during upload validation.
func (a *AdminPlugins) runPluginAction(w http.ResponseWriter, r *http.Request, id, action string) error {
	switch action {
	case "enable":
		return a.manager.Enable(r.Context(), id)
	case "disable":
		return a.manager.Disable(r.Context(), id)
	case "uninstall":
		return a.manager.Uninstall(r.Context(), id)
	case "update":
		return a.updateFromCatalog(r.Context(), id)
	case "upgrade":
		return a.upgradeFromUpload(w, r, id)
	default:
		http.NotFound(w, r)
		return errPluginActionHandled
	}
}

// upgradeFromUpload validates an uploaded package identity before replacing the installed plugin.
func (a *AdminPlugins) upgradeFromUpload(w http.ResponseWriter, r *http.Request, id string) error {
	archive, status, err := readPluginUpload(w, r)
	if err != nil {
		a.render(w, r, pluginDetailID(r, id), status, "Upload failed: "+err.Error()+".")
		return errPluginActionHandled
	}
	pkg, err := pluginpackage.Read(archive)
	if err != nil {
		a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "Invalid plugin package: "+err.Error())
		return errPluginActionHandled
	}
	if pkg.Manifest().ID != id {
		a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "The uploaded package must have the same plugin ID.")
		return errPluginActionHandled
	}
	_, err = a.manager.Upgrade(r.Context(), id, archive)
	return err
}

// pluginActionDestination returns the post-action administration destination.
func pluginActionDestination(r *http.Request, id, action string) string {
	if action != "uninstall" && pluginDetailID(r, id) != "" {
		return "/admin/plugins?plugin=" + id
	}
	return "/admin/plugins"
}

// updateFromCatalog downloads and applies the newest compatible catalog release for id.
func (a *AdminPlugins) updateFromCatalog(ctx context.Context, id string) error {
	if a.updates == nil {
		return errors.New("plugin update catalog is unavailable")
	}

	if !pluginInstalled(a.manager.Plugins(), id) {
		return errors.New("plugin is not installed")
	}

	updates, err := a.updates.Available()
	if err != nil {
		return fmt.Errorf("check plugin update catalog: %w", err)
	}
	release, ok := updates[id]
	if !ok {
		return errors.New("no newer compatible plugin release is available")
	}

	archive, err := a.updates.Download(ctx, id, release.Version)
	if err != nil {
		return fmt.Errorf("download plugin update: %w", err)
	}
	_, err = a.manager.Upgrade(ctx, id, archive)
	return err
}

// updateAllFromCatalog downloads and applies every newer compatible installed plugin release.
func (a *AdminPlugins) updateAllFromCatalog(ctx context.Context) ([]string, error) {
	if a.updates == nil {
		return nil, errors.New("plugin update catalog is unavailable")
	}
	updates, err := a.updates.Available()
	if err != nil {
		return nil, fmt.Errorf("check plugin update catalog: %w", err)
	}
	ids := installedUpdateIDs(a.manager.Plugins(), updates)
	pending, err := a.downloadPendingUpdates(ctx, ids, updates)
	if err != nil {
		return nil, err
	}
	return a.applyPendingUpdates(ctx, pending)
}

// installedUpdateIDs returns sorted catalog update IDs that are currently installed.
func installedUpdateIDs(installed []plugin.LoadedPlugin, updates map[string]domain.PluginRelease) []string {
	installedIDs := make(map[string]bool, len(installed))
	for _, item := range installed {
		installedIDs[item.Manifest.ID] = true
	}
	ids := make([]string, 0, len(updates))
	for id := range updates {
		if installedIDs[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// downloadPendingUpdates downloads and validates every package before any upgrade is applied.
func (a *AdminPlugins) downloadPendingUpdates(ctx context.Context, ids []string, updates map[string]domain.PluginRelease) ([]pendingPluginUpdate, error) {
	pending := make([]pendingPluginUpdate, 0, len(ids))
	for _, id := range ids {
		release := updates[id]
		archive, err := a.updates.Download(ctx, id, release.Version)
		if err != nil {
			return nil, fmt.Errorf("download %s update: %w", id, err)
		}
		pkg, err := pluginpackage.Read(archive)
		if err != nil {
			return nil, fmt.Errorf("validate %s update: %w", id, err)
		}
		if pkg.Manifest().ID != id {
			return nil, fmt.Errorf("validate %s update: package identity mismatch", id)
		}
		pending = append(pending, pendingPluginUpdate{id: id, archive: archive})
	}
	return pending, nil
}

// applyPendingUpdates upgrades validated packages in deterministic order and reports partial progress.
func (a *AdminPlugins) applyPendingUpdates(ctx context.Context, pending []pendingPluginUpdate) ([]string, error) {
	updated := make([]string, 0, len(pending))
	for _, item := range pending {
		if _, err := a.manager.Upgrade(ctx, item.id, item.archive); err != nil {
			return updated, fmt.Errorf("upgrade %s: %w", item.id, err)
		}
		updated = append(updated, item.id)
	}
	return updated, nil
}

// pluginInstalled reports whether the manager snapshot contains id.
func pluginInstalled(items []plugin.LoadedPlugin, id string) bool {
	for _, item := range items {
		if item.Manifest.ID == id {
			return true
		}
	}

	return false
}

// renderPluginREADME renders package documentation without activating plugin macros.
func renderPluginREADME(source string) (template.HTML, error) {
	source = strings.TrimSpace(source)
	if strings.HasPrefix(source, "# ") {
		if _, rest, ok := strings.Cut(source, "\n"); ok {
			source = strings.TrimSpace(rest)
		}
	}

	renderer := md.NewWithRegistry(nil)
	rendered, err := renderer.Render(source)
	if err != nil {
		return "", err
	}
	return template.HTML(rendered), nil
}

// failure records a plugin administration failure and redirects the request.
func (a *AdminPlugins) failure(w http.ResponseWriter, r *http.Request, id, action string, err error) {
	a.views.Logger().Error("plugin administration failed", "action", action, "plugin_id", id, "error", err)
	// Runtime/storage errors can contain implementation details. Keep them in logs.
	message := "Could not " + action + " the plugin. Check its dependencies, requested permissions, and system-plugin restrictions. The existing plugin state was preserved."
	if action == "update" {
		message = "Could not update the plugin from the Kumbuka catalog. The existing version was preserved; manual package upgrade remains available."
	}
	a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, message)
}

// pluginDetailID returns id when the request originated from a plugin detail modal.
func pluginDetailID(r *http.Request, id string) string {
	if r.URL.Query().Get("return") == "detail" {
		return id
	}
	return ""
}

// audit records a successful plugin administration action.
func (a *AdminPlugins) audit(r *http.Request, action, id string) {
	a.views.Logger().Info("plugin lifecycle changed", "event", "plugin."+action, "plugin_id", id, "actor_id", currentUser(r).ID)
}

// readPluginUpload streams one bounded package without temporary files or extraction.
func readPluginUpload(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, pluginpackage.MaxArchiveBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("choose a .kumbukaplugin package to upload")
	}
	part, err := reader.NextPart()
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("upload exactly one plugin package")
	}
	if !validPluginUploadPart(part.FormName(), part.FileName()) {
		return nil, http.StatusBadRequest, errors.New("upload exactly one plugin package")
	}
	content, err := io.ReadAll(io.LimitReader(part, pluginpackage.MaxArchiveBytes+1))
	if len(content) > pluginpackage.MaxArchiveBytes {
		return nil, http.StatusRequestEntityTooLarge, errors.New("plugin packages must be 16 MiB or smaller")
	}
	if err != nil {
		return nil, http.StatusBadRequest, errors.New("could not read the plugin package")
	}
	if _, err := reader.NextPart(); err != io.EOF {
		return nil, http.StatusBadRequest, errors.New("upload exactly one plugin package")
	}
	return content, http.StatusOK, nil
}
