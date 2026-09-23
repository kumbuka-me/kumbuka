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

// render renders the plugin administration page.
func (a *AdminPlugins) render(w http.ResponseWriter, r *http.Request, id string, status int, message string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	layout, err := administrationData(r, a.data, a.views, "Plugins", "plugins")
	data := webview.AdminPluginsView{Layout: layout}
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	data.AdminPlugins = a.manager.Plugins()
	data.PluginRequiredIDs = make(map[string]bool, len(data.AdminPlugins))
	data.PluginUpdates = make(map[string]*webview.PluginUpdate)
	if a.updates == nil {
		data.PluginCatalogUnavailable = true
	} else {
		status := a.updates.Status()
		data.PluginUpdateStatus = webview.PluginUpdateStatus{
			Available:   true,
			Automatic:   status.Automatic,
			LastAttempt: status.LastAttempt,
			LastSuccess: status.LastSuccess,
			LastError:   status.LastError,
		}
		updates, updateErr := a.updates.Available()
		if updateErr != nil {
			data.PluginCatalogUnavailable = true
		} else {
			for pluginID, release := range updates {
				data.PluginUpdates[pluginID] = &webview.PluginUpdate{Version: release.Version, ReleasedAt: release.ReleasedAt}
			}
		}
	}
	data.PluginHasSettings = make(map[string]bool, len(data.AdminPlugins))
	data.PluginREADMEs = make(map[string]template.HTML, len(data.AdminPlugins))
	foundOpenPlugin := id == ""
	for _, item := range data.AdminPlugins {
		pluginID := item.Manifest.ID
		data.PluginRequiredIDs[pluginID] = a.manager.IsRequired(pluginID)
		for _, module := range item.Manifest.Modules {
			if module.Type == "settings" || module.Type == "admin-resource" {
				data.PluginHasSettings[pluginID] = true
				break
			}
		}
		readme, renderErr := renderPluginREADME(item.README)
		if renderErr != nil {
			httpresponse.InternalServerError(a.views.Logger(), w, renderErr)
			return
		}
		data.PluginREADMEs[pluginID] = readme
		if pluginID == id {
			foundOpenPlugin = true
		}
	}
	if !foundOpenPlugin {
		http.NotFound(w, r)
		return
	}
	sort.Slice(data.AdminPlugins, func(i, j int) bool { return data.AdminPlugins[i].Manifest.Name < data.AdminPlugins[j].Manifest.Name })
	data.OpenPluginID = id
	data.PluginMessage = message
	a.views.RenderStatus(w, status, "admin_plugins", data)
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
		updated, err := a.updateAllFromCatalog(r.Context())
		if err != nil {
			a.views.Logger().Error("bulk plugin update failed", "event", "plugin.update_all_failed", "error", err, "actor_id", currentUser(r).ID)
			a.render(w, r, "", http.StatusUnprocessableEntity, "Could not update all plugins from the Kumbuka catalog. Plugins updated before the failure remain on their new versions; the remaining plugins were left unchanged.")
			return
		}
		for _, pluginID := range updated {
			a.audit(r, "update", pluginID)
		}
		http.Redirect(w, r, "/admin/plugins", http.StatusSeeOther)
		return
	}

	var err error
	switch action {
	case "enable":
		err = a.manager.Enable(r.Context(), id)
	case "disable":
		err = a.manager.Disable(r.Context(), id)
	case "uninstall":
		err = a.manager.Uninstall(r.Context(), id)
	case "update":
		err = a.updateFromCatalog(r.Context(), id)
	case "upgrade":
		var archive []byte
		var status int
		archive, status, err = readPluginUpload(w, r)
		if err != nil {
			a.render(w, r, pluginDetailID(r, id), status, "Upload failed: "+err.Error()+".")
			return
		}
		pkg, parseErr := pluginpackage.Read(archive)
		if parseErr != nil {
			a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "Invalid plugin package: "+parseErr.Error())
			return
		}
		if pkg.Manifest().ID != id {
			a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "The uploaded package must have the same plugin ID.")
			return
		}
		_, err = a.manager.Upgrade(r.Context(), id, archive)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.failure(w, r, id, action, err)
		return
	}
	a.audit(r, action, id)
	destination := "/admin/plugins"
	if action != "uninstall" && pluginDetailID(r, id) != "" {
		destination += "?plugin=" + id
	}
	http.Redirect(w, r, destination, http.StatusSeeOther)
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
	if len(updates) == 0 {
		return nil, nil
	}

	installed := make(map[string]bool, len(a.manager.Plugins()))
	for _, item := range a.manager.Plugins() {
		installed[item.Manifest.ID] = true
	}

	ids := make([]string, 0, len(updates))
	for id := range updates {
		if installed[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)

	// pendingUpdate keeps one validated catalog package ready for installation.
	type pendingUpdate struct {
		// id identifies the installed plugin to upgrade.
		id string
		// archive contains the validated plugin package bytes.
		archive []byte
	}
	pending := make([]pendingUpdate, 0, len(ids))
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
		pending = append(pending, pendingUpdate{id: id, archive: archive})
	}

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
