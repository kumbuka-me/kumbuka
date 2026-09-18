package handler

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// pluginUpdateService is the first-party catalog boundary used by plugin administration.
type pluginUpdateService interface {
	// Updates returns newer compatible releases keyed by installed plugin ID.
	Updates(context.Context, map[string]string) (map[string]pluginupdate.Release, error)
	// Download retrieves and verifies one selected plugin release.
	Download(context.Context, string, pluginupdate.Release) ([]byte, error)
}

// AdminPlugins exposes package metadata and lifecycle operations through the
// existing administration layout. Routes apply browser authentication/admin authorization.
type AdminPlugins struct {
	// manager owns active plugin lifecycle state.
	manager *plugin.Manager
	// updates discovers and downloads compatible first-party plugin releases.
	updates pluginUpdateService
	// data loads shared administration view data.
	data viewDataService
	// views renders plugin administration responses.
	views *webview.Views
}

// NewAdminPlugins constructs the plugin administration handler.
func NewAdminPlugins(manager *plugin.Manager, updates pluginUpdateService, data viewDataService, views *webview.Views) *AdminPlugins {
	return &AdminPlugins{manager: manager, updates: updates, data: data, views: views}
}

// List renders the plugin inventory and optionally opens one plugin detail modal.
func (a *AdminPlugins) List(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, strings.TrimSpace(r.URL.Query().Get("plugin")), http.StatusOK, "")
}

// render renders the plugin administration page.
func (a *AdminPlugins) render(w http.ResponseWriter, r *http.Request, id string, status int, message string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	data, err := administrationData(r, a.data, a.views, "Plugins", "plugins")
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	data.AdminPlugins = a.manager.Plugins()
	data.PluginRequiredIDs = make(map[string]bool, len(data.AdminPlugins))
	data.PluginUpdates = make(map[string]*webview.PluginUpdate)
	data.PluginUpdatesEnabled = a.updates != nil
	if a.updates != nil {
		updates, updateErr := a.updates.Updates(r.Context(), pluginVersions(data.AdminPlugins))
		if updateErr != nil {
			data.PluginCatalogUnavailable = true
			a.views.Logger().Warn("check plugin updates", "event", "plugin_catalog_check_failed", "error", updateErr)
		} else {
			for pluginID, release := range updates {
				data.PluginUpdates[pluginID] = &webview.PluginUpdate{Version: release.Version, ReleasedAt: release.ReleasedAt}
			}
		}
	}
	data.PluginHasSettings = make(map[string]bool, len(data.AdminPlugins))
	data.PluginREADMEs = make(map[string]template.HTML, len(data.AdminPlugins))
	data.PluginResources = make(map[string][]webview.PluginResource, len(data.AdminPlugins))
	foundOpenPlugin := id == ""
	for _, item := range data.AdminPlugins {
		pluginID := item.Manifest.ID
		data.PluginRequiredIDs[pluginID] = a.manager.IsRequired(pluginID)
		for _, module := range item.Manifest.Modules {
			switch module.Type {
			case "settings":
				data.PluginHasSettings[pluginID] = true
			case "admin-resource":
				records, resourceErr := a.manager.ResourceRecords(r.Context(), pluginID, module.ID)
				if resourceErr != nil {
					httpresponse.InternalServerError(a.views.Logger(), w, resourceErr)
					return
				}
				data.PluginResources[pluginID] = append(data.PluginResources[pluginID], webview.PluginResource{Module: module, Records: records})
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
	var err error
	switch action {
	case "enable":
		err = a.manager.Enable(r.Context(), id)
	case "disable":
		err = a.manager.Disable(r.Context(), id)
	case "settings":
		err = a.updateSettings(r, id)
	case "resource-save":
		err = a.saveResource(r, id)
	case "resource-delete":
		err = a.deleteResource(r, id)
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

	versions := pluginVersions(a.manager.Plugins())
	current, ok := versions[id]
	if !ok {
		return errors.New("plugin is not installed")
	}

	updates, err := a.updates.Updates(ctx, map[string]string{id: current})
	if err != nil {
		return fmt.Errorf("check plugin update catalog: %w", err)
	}
	release, ok := updates[id]
	if !ok {
		return errors.New("no newer compatible plugin release is available")
	}

	archive, err := a.updates.Download(ctx, id, release)
	if err != nil {
		return fmt.Errorf("download plugin update: %w", err)
	}
	_, err = a.manager.Upgrade(ctx, id, archive)
	return err
}

// pluginVersions indexes installed plugin versions by stable plugin ID.
func pluginVersions(items []plugin.LoadedPlugin) map[string]string {
	versions := make(map[string]string, len(items))
	for _, item := range items {
		versions[item.Manifest.ID] = item.Manifest.Version
	}
	return versions
}

// updateSettings persists every declared boolean setting for one plugin.
func (a *AdminPlugins) updateSettings(r *http.Request, id string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}

	var selected *plugin.LoadedPlugin
	for _, item := range a.manager.Plugins() {
		if item.Manifest.ID == id {
			copy := item
			selected = &copy
			break
		}
	}
	if selected == nil {
		return errors.New("plugin is not installed")
	}

	settings := make(map[string]bool)
	for _, module := range selected.Manifest.Modules {
		if module.Type == "settings" {
			settings[module.ID] = r.Form.Has("setting_" + module.ID)
		}
	}

	return a.manager.UpdateSettings(r.Context(), id, settings)
}

// saveResource validates and persists one generic plugin-owned admin resource record.
func (a *AdminPlugins) saveResource(r *http.Request, id string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	moduleID := strings.TrimSpace(r.FormValue("resource_id"))
	var module *pluginpackage.Module
	for _, item := range a.manager.Plugins() {
		if item.Manifest.ID != id {
			continue
		}
		for index := range item.Manifest.Modules {
			if item.Manifest.Modules[index].ID == moduleID && item.Manifest.Modules[index].Type == "admin-resource" {
				copy := item.Manifest.Modules[index]
				module = &copy
				break
			}
		}
	}
	if module == nil {
		return errors.New("plugin resource is not declared")
	}
	values := make(map[string]string, len(module.Fields))
	for _, field := range module.Fields {
		values[field.ID] = r.FormValue("resource_" + field.ID)
	}
	return a.manager.SaveResourceRecord(r.Context(), id, moduleID, r.FormValue("original_key"), values)
}

// deleteResource removes one generic plugin-owned admin resource record.
func (a *AdminPlugins) deleteResource(r *http.Request, id string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	return a.manager.DeleteResourceRecord(r.Context(), id, strings.TrimSpace(r.FormValue("resource_id")), r.FormValue("record_key"))
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
	if action == "settings" {
		message = "Could not save plugin settings. Check the setting dependencies and try again."
	}
	if action == "resource-save" || action == "resource-delete" {
		message = "Could not update plugin data. Check the entered values and try again."
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
	if err != nil || part.FormName() != "package" || part.FileName() == "" {
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
