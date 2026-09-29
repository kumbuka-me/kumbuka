package endpoint

import (
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strings"

	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/route"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// validPluginUploadPart reports whether a multipart part is the required named plugin package file.
func validPluginUploadPart(formName, filename string) bool {
	return formName == "package" && filename != ""
}

// AdminPlugins exposes package metadata and lifecycle operations through the existing administration layout. Routes apply browser authentication/admin authorization.
type AdminPlugins struct {
	// manager coordinates lifecycle actions and catalog updates in the application layer.
	manager *appplugins.Admin
	// data loads shared administration view data.
	data browserContextLoader
	// views renders plugin administration responses.
	views *webview.Views
	// renders coordinates background rebuilds of persisted page-render artifacts.
	renders *AdminRenderRebuilds
}

// NewAdminPlugins constructs the plugin administration handler.
func NewAdminPlugins(manager *appplugins.Admin, data browserContextLoader, views *webview.Views) *AdminPlugins {
	return &AdminPlugins{manager: manager, data: data, views: views}
}

// WithRenderRebuilds enables page render-cache rebuilds after render-affecting plugin lifecycle changes.
func (a *AdminPlugins) WithRenderRebuilds(rebuilds *AdminRenderRebuilds) *AdminPlugins {
	a.renders = rebuilds
	return a
}

// List renders the plugin inventory and optionally opens one plugin detail modal.
func (a *AdminPlugins) List(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, strings.TrimSpace(r.URL.Query().Get("plugin")), http.StatusOK, "")
}

// CheckUpdates refreshes the first-party plugin catalog immediately.
func (a *AdminPlugins) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	if a.manager == nil || !a.manager.CatalogAvailable() {
		a.render(w, r, "", http.StatusServiceUnavailable, "Plugin update checks are unavailable.")
		return
	}
	if err := a.manager.Refresh(r.Context()); err != nil {
		a.views.Logger().Warn("check plugin updates", "event", "plugin_catalog_manual_check_failed", "error", err, "actor_id", currentUser(r).ID)
		a.render(w, r, "", http.StatusBadGateway, "Could not check the plugin update catalog. The previous successful catalog remains available if one exists.")
		return
	}

	a.views.Logger().Info("plugin update catalog checked", "event", "plugin.catalog_check", "actor_id", currentUser(r).ID)
	route.Redirect(w, r, "/admin/plugins", http.StatusSeeOther)
}

// pluginPermissionApprovalState carries operation-specific review prompts into one plugin administration render.
type pluginPermissionApprovalState struct {
	catalog  map[string]*webview.PluginPermissionApproval
	install  *webview.PluginPermissionApproval
	upgrades map[string]*webview.PluginPermissionApproval
}

// render renders the plugin administration page.
func (a *AdminPlugins) render(w http.ResponseWriter, r *http.Request, id string, status int, message string) {
	a.renderWithPermissionApprovals(w, r, id, status, message, pluginPermissionApprovalState{})
}

// renderWithPermissionApprovals renders plugin administration with operation-specific permission review prompts.
func (a *AdminPlugins) renderWithPermissionApprovals(
	w http.ResponseWriter,
	r *http.Request,
	id string,
	status int,
	message string,
	approvals pluginPermissionApprovalState,
) {
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
	data := webview.AdminPluginsView{
		Layout:                    layout,
		AdminPlugins:              a.manager.Plugins(),
		PluginPermissionApprovals: approvals.catalog,
		PluginInstallApproval:     approvals.install,
		PluginUpgradeApprovals:    approvals.upgrades,
	}
	a.populatePluginUpdateView(&data)
	if err := a.populatePluginDetails(&data, id); err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}
	if id != "" && !a.manager.HasPlugin(id) {
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
	if !a.manager.CatalogAvailable() {
		data.PluginCatalogUnavailable = true
		return
	}
	status := a.manager.Status()
	data.PluginUpdateStatus = webview.PluginUpdateStatus{
		Available: true, Automatic: status.Automatic, LastAttempt: status.LastAttempt, LastSuccess: status.LastSuccess, LastError: status.LastError,
	}
	updates, err := a.manager.Available()
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
		if isPluginSettingsModule(module.Type) {
			return true
		}
	}
	return false
}

// isPluginSettingsModule reports whether a module contributes settings or administration resources.
func isPluginSettingsModule(moduleType string) bool {
	switch plugin.ModuleType(moduleType) {
	case plugin.ModuleTypeSettings, plugin.ModuleTypeAdminResource:
		return true
	default:
		return false
	}
}

// Install reviews and installs one uploaded plugin package. Every new installation requires explicit administrator confirmation, even when the package requests no permissions.
func (a *AdminPlugins) Install(w http.ResponseWriter, r *http.Request) {
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}
	upload, status, err := readPluginUpload(w, r)
	if err != nil {
		a.render(w, r, "", status, "Upload failed: "+err.Error()+".")
		return
	}
	if _, err = pluginpackage.Read(upload.archive); err != nil {
		a.render(w, r, "", http.StatusUnprocessableEntity, "Invalid plugin package: "+err.Error())
		return
	}
	item, err := a.manager.Install(r.Context(), upload.archive, upload.approvals...)
	if err != nil {
		var approval *appplugins.PermissionApprovalRequiredError
		if errors.As(err, &approval) {
			a.renderPermissionApproval(w, r, approval.PluginID, approval)
			return
		}
		a.failure(w, r, "", "install", err)
		return
	}
	if item.Enabled && md.PluginAffectsArtifact(item.Manifest) && a.renders != nil {
		a.renders.QueueAll("render-affecting plugin installed: " + item.Manifest.ID)
	}
	a.audit(r, "install", item.Manifest.ID)
	route.Redirect(w, r, "/admin/plugins?plugin="+item.Manifest.ID, http.StatusSeeOther)
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

	before, beforeOK := a.pluginSnapshot(id)
	if err := a.runPluginAction(w, r, id, action); err != nil {
		if errors.Is(err, errPluginActionHandled) {
			return
		}
		var approval *appplugins.PermissionApprovalRequiredError
		if errors.As(err, &approval) {
			a.renderPermissionApproval(w, r, id, approval)
			return
		}
		a.failure(w, r, id, action, err)
		return
	}
	after, afterOK := a.pluginSnapshot(id)
	if pluginRenderStateChanged(before, beforeOK, after, afterOK) {
		a.scheduleRenderRebuild(r, "render-affecting plugin "+action+": "+id)
	}

	a.audit(r, action, id)
	route.Redirect(w, r, pluginActionDestination(r, id, action), http.StatusSeeOther)
}

var errPluginActionHandled = errors.New("plugin action response already written")

// handleBulkUpdate applies all catalog updates independently so one blocked or failed plugin never stops the rest.
func (a *AdminPlugins) handleBulkUpdate(w http.ResponseWriter, r *http.Request) {
	before := pluginInventoryByID(a.manager.Plugins())
	result, err := a.manager.UpdateAll(r.Context())
	if err != nil {
		a.views.Logger().Error("bulk plugin update failed", "event", "plugin.update_all_failed", "error", err, "actor_id", currentUser(r).ID)
		a.render(w, r, "", http.StatusUnprocessableEntity, "Could not check or download updates from the Kumbuka catalog. No remaining plugin updates were attempted.")
		return
	}

	for _, pluginID := range result.Updated {
		a.audit(r, "update", pluginID)
	}

	after := pluginInventoryByID(a.manager.Plugins())
	for _, pluginID := range result.Updated {
		previous, previousOK := before[pluginID]
		current, currentOK := after[pluginID]
		if pluginRenderStateChanged(previous, previousOK, current, currentOK) {
			if a.renders != nil {
				a.renders.QueueAll("render-affecting plugin update batch")
			}
			break
		}
	}

	if len(result.Failed) == 0 {
		route.Redirect(w, r, "/admin/plugins", http.StatusSeeOther)
		return
	}

	approvals := make(map[string]*webview.PluginPermissionApproval)
	approvalCount := 0
	failureCount := 0
	for _, failure := range result.Failed {
		var approval *appplugins.PermissionApprovalRequiredError
		if errors.As(failure.Err, &approval) {
			approvals[failure.PluginID] = pluginPermissionApprovalView(approval)
			approvalCount++
			continue
		}
		failureCount++
		a.views.Logger().Error(
			"plugin update failed during bulk update",
			"event", "plugin.update_failed",
			"plugin_id", failure.PluginID,
			"error", failure.Err,
			"actor_id", currentUser(r).ID,
		)
	}

	a.views.Logger().Warn(
		"bulk plugin update completed with attention required",
		"event", "plugin.update_all_partial",
		"updated", len(result.Updated),
		"permission_approvals", approvalCount,
		"failed", failureCount,
		"actor_id", currentUser(r).ID,
	)
	a.renderWithPermissionApprovals(
		w, r, "", http.StatusUnprocessableEntity, bulkPluginUpdateMessage(len(result.Updated), approvalCount, failureCount),
		pluginPermissionApprovalState{catalog: approvals},
	)
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
		return a.manager.Update(r.Context(), id, pluginUpdateApproval(r)...)
	case "upgrade":
		return a.upgradeFromUpload(w, r, id)
	default:
		http.NotFound(w, r)
		return errPluginActionHandled
	}
}

// pluginUpdateApproval returns exact package and permission consent submitted from a server-rendered catalog approval card.
func pluginUpdateApproval(r *http.Request) []appplugins.UpdateApproval {
	operation := strings.TrimSpace(r.FormValue("approve_operation"))
	pluginID := strings.TrimSpace(r.FormValue("approve_plugin_id"))
	version := strings.TrimSpace(r.FormValue("approve_version"))
	digest := strings.TrimSpace(r.FormValue("approve_digest"))
	if operation == "" || pluginID == "" || version == "" || digest == "" {
		return nil
	}
	permissions := append([]string(nil), r.Form["approve_permission"]...)
	return []appplugins.UpdateApproval{{Operation: operation, PluginID: pluginID, Version: version, Digest: digest, Permissions: permissions}}
}

// upgradeFromUpload validates an uploaded package identity before replacing the installed plugin. Permission-set changes are returned to Action for explicit administrator review.
func (a *AdminPlugins) upgradeFromUpload(w http.ResponseWriter, r *http.Request, id string) error {
	upload, status, err := readPluginUpload(w, r)
	if err != nil {
		a.render(w, r, pluginDetailID(r, id), status, "Upload failed: "+err.Error()+".")
		return errPluginActionHandled
	}
	pkg, err := pluginpackage.Read(upload.archive)
	if err != nil {
		a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "Invalid plugin package: "+err.Error())
		return errPluginActionHandled
	}
	if pkg.Manifest().ID != id {
		a.render(w, r, pluginDetailID(r, id), http.StatusUnprocessableEntity, "The uploaded package must have the same plugin ID.")
		return errPluginActionHandled
	}
	return a.manager.Upgrade(r.Context(), id, upload.archive, upload.approvals...)
}

// pluginActionDestination returns the post-action administration destination.
func pluginActionDestination(r *http.Request, id, action string) string {
	if action != "uninstall" && pluginDetailID(r, id) != "" {
		return "/admin/plugins?plugin=" + id
	}
	return "/admin/plugins"
}

// pluginSnapshot returns one loaded plugin from the current manager inventory.
func (a *AdminPlugins) pluginSnapshot(id string) (plugin.LoadedPlugin, bool) {
	if a == nil || a.manager == nil {
		return plugin.LoadedPlugin{}, false
	}
	for _, item := range a.manager.Plugins() {
		if item.Manifest.ID == id {
			return item, true
		}
	}
	return plugin.LoadedPlugin{}, false
}

// pluginInventoryByID indexes one plugin inventory by stable plugin ID.
func pluginInventoryByID(items []plugin.LoadedPlugin) map[string]plugin.LoadedPlugin {
	result := make(map[string]plugin.LoadedPlugin, len(items))
	for _, item := range items {
		result[item.Manifest.ID] = item
	}
	return result
}

// pluginRenderStateChanged reports whether a lifecycle operation changed an enabled render-affecting plugin.
func pluginRenderStateChanged(before plugin.LoadedPlugin, beforeOK bool, after plugin.LoadedPlugin, afterOK bool) bool {
	beforeAffects := beforeOK && before.Enabled && md.PluginAffectsArtifact(before.Manifest)
	afterAffects := afterOK && after.Enabled && md.PluginAffectsArtifact(after.Manifest)
	if !beforeAffects && !afterAffects {
		return false
	}
	if beforeOK != afterOK {
		return true
	}
	return before.Enabled != after.Enabled ||
		before.Manifest.Version != after.Manifest.Version ||
		!reflect.DeepEqual(before.Digest, after.Digest)
}

// scheduleRenderRebuild either defers a bulk-progress rebuild or starts the background rebuild immediately.
func (a *AdminPlugins) scheduleRenderRebuild(r *http.Request, reason string) {
	if a.renders == nil {
		return
	}
	if r.URL.Query().Get("defer_render") == "1" {
		a.renders.MarkAllDirty(reason)
		return
	}
	a.renders.QueueAll(reason)
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

// renderPermissionApproval keeps the existing state unchanged and asks the administrator to review the exact package permission request before continuing.
func (a *AdminPlugins) renderPermissionApproval(w http.ResponseWriter, r *http.Request, id string, approval *appplugins.PermissionApprovalRequiredError) {
	a.views.Logger().Info(
		"plugin package requires permission approval",
		"event", "plugin.permission_approval_required",
		"operation", approval.Operation,
		"plugin_id", approval.PluginID,
		"version", approval.Version,
		"permissions", approval.Permissions,
		"added_permissions", approval.AddedPermissions,
		"removed_permissions", approval.RemovedPermissions,
		"actor_id", currentUser(r).ID,
	)

	view := pluginPermissionApprovalView(approval)
	state := pluginPermissionApprovalState{}
	openPluginID := pluginDetailID(r, id)
	message := "This update changes the plugin permission set. Review and confirm the exact package before updating; the existing plugin version is still active."
	switch approval.Operation {
	case "install":
		state.install = view
		openPluginID = ""
		message = "Review this plugin's requested permissions before installing it. Nothing has been installed yet."
	case "upgrade":
		state.upgrades = map[string]*webview.PluginPermissionApproval{id: view}
		message = "This uploaded package changes the plugin permission set. Review and confirm the change before upgrading; the existing plugin version is still active."
	default:
		state.catalog = map[string]*webview.PluginPermissionApproval{id: view}
	}

	a.renderWithPermissionApprovals(w, r, openPluginID, http.StatusConflict, message, state)
}

// pluginPermissionApprovalView converts an application permission challenge into presentation data.
func pluginPermissionApprovalView(approval *appplugins.PermissionApprovalRequiredError) *webview.PluginPermissionApproval {
	return &webview.PluginPermissionApproval{
		Operation:          approval.Operation,
		PluginID:           approval.PluginID,
		Name:               approval.Name,
		Provider:           approval.Provider,
		Description:        approval.Description,
		Version:            approval.Version,
		Digest:             approval.Digest,
		Permissions:        pluginPermissionViews(approval.Permissions),
		AddedPermissions:   pluginPermissionViews(approval.AddedPermissions),
		RemovedPermissions: pluginPermissionViews(approval.RemovedPermissions),
	}
}

// pluginPermissionViews decorates stable capability identifiers with administrator-facing descriptions.
func pluginPermissionViews(permissions []string) []webview.PluginPermission {
	result := make([]webview.PluginPermission, 0, len(permissions))
	for _, permission := range permissions {
		result = append(result, webview.PluginPermission{Name: permission, Description: pluginPermissionDescription(permission)})
	}
	return result
}

// pluginPermissionDescription explains host-mediated permissions without implying broader access than the capability provides.
func pluginPermissionDescription(permission string) string {
	switch permission {
	case "pages:write":
		return "May update the current page through Kumbuka's normal page edit authorization and concurrency checks."
	case "pages:content":
		return "May read Markdown content for pages made available to the plugin by Kumbuka."
	case "pages:read":
		return "May read page metadata and page relationships made available to the plugin by Kumbuka."
	case "browser:render":
		return "May contribute isolated browser-rendered plugin UI."
	case "network:http":
		return "May make outbound HTTP requests through Kumbuka's bounded network adapter."
	case "network:private":
		return "May allow approved HTTP requests to private network destinations."
	case "network:insecure-tls":
		return "May disable origin TLS verification for plugin HTTP requests."
	case "storage:read":
		return "May read data stored in this plugin's own namespace."
	case "storage:write":
		return "May write data in this plugin's own namespace."
	case "settings:read":
		return "May read this plugin's own declared settings."
	case "settings:write":
		return "May update this plugin's own declared settings."
	case "users:read":
		return "May resolve the bounded public user information exposed to plugins."
	case "notifications:send":
		return "May create host-attributed notifications through Kumbuka."
	case "activity:read":
		return "May read the bounded activity information exposed to plugins."
	case "drafts:read":
		return "May read the bounded draft information exposed to plugins."
	default:
		return "Requests a host-mediated Kumbuka capability."
	}
}

// bulkPluginUpdateMessage summarizes independent bulk outcomes while keeping per-plugin technical errors out of the browser.
func bulkPluginUpdateMessage(updated, approvals, failed int) string {
	parts := make([]string, 0, 3)
	if updated > 0 {
		parts = append(parts, fmt.Sprintf("Updated %d plugin(s).", updated))
	}
	if approvals > 0 {
		parts = append(parts, fmt.Sprintf("%d update(s) require permission approval; open those plugins to review the permission changes.", approvals))
	}
	if failed > 0 {
		parts = append(parts, fmt.Sprintf("%d update(s) failed and can be retried without affecting the successful updates.", failed))
	}
	return strings.Join(parts, " ")
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

// pluginPackageUpload contains one bounded package plus any exact permission approval fields submitted alongside it.
type pluginPackageUpload struct {
	archive   []byte
	approvals []appplugins.PermissionApproval
}

// readPluginUpload streams one bounded package and its small approval fields without temporary files or extraction.
func readPluginUpload(w http.ResponseWriter, r *http.Request) (pluginPackageUpload, int, error) {
	r.Body = http.MaxBytesReader(w, r.Body, pluginpackage.MaxArchiveBytes+(64<<10))
	reader, err := r.MultipartReader()
	if err != nil {
		return pluginPackageUpload{}, http.StatusBadRequest, errors.New("choose a .kumbukaplugin package to upload")
	}

	var archive []byte
	fields := make(map[string][]string)
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return pluginPackageUpload{}, http.StatusBadRequest, errors.New("could not read the plugin upload")
		}

		if part.FileName() != "" {
			if archive != nil || !validPluginUploadPart(part.FormName(), part.FileName()) {
				return pluginPackageUpload{}, http.StatusBadRequest, errors.New("upload exactly one plugin package")
			}
			content, readErr := io.ReadAll(io.LimitReader(part, pluginpackage.MaxArchiveBytes+1))
			if len(content) > pluginpackage.MaxArchiveBytes {
				return pluginPackageUpload{}, http.StatusRequestEntityTooLarge, errors.New("plugin packages must be 16 MiB or smaller")
			}
			if readErr != nil {
				return pluginPackageUpload{}, http.StatusBadRequest, errors.New("could not read the plugin package")
			}
			archive = content
			continue
		}

		name := part.FormName()
		if !validPluginApprovalField(name) {
			return pluginPackageUpload{}, http.StatusBadRequest, errors.New("plugin upload contains an unexpected form field")
		}
		value, readErr := io.ReadAll(io.LimitReader(part, 4097))
		if readErr != nil || len(value) > 4096 {
			return pluginPackageUpload{}, http.StatusBadRequest, errors.New("plugin approval field is invalid")
		}
		fields[name] = append(fields[name], strings.TrimSpace(string(value)))
	}

	if archive == nil {
		return pluginPackageUpload{}, http.StatusBadRequest, errors.New("upload exactly one plugin package")
	}
	return pluginPackageUpload{archive: archive, approvals: pluginPermissionApprovals(fields)}, http.StatusOK, nil
}

// validPluginApprovalField limits multipart metadata to the exact fields emitted by Kumbuka's review UI.
func validPluginApprovalField(name string) bool {
	switch name {
	case "approve_operation", "approve_plugin_id", "approve_version", "approve_digest", "approve_permission":
		return true
	default:
		return false
	}
}

// pluginPermissionApprovals decodes one exact package approval from form values. Partial approval metadata is ignored and causes a fresh review challenge.
func pluginPermissionApprovals(fields map[string][]string) []appplugins.PermissionApproval {
	first := func(name string) string {
		values := fields[name]
		if len(values) == 0 {
			return ""
		}
		return strings.TrimSpace(values[0])
	}
	operation := first("approve_operation")
	pluginID := first("approve_plugin_id")
	version := first("approve_version")
	digest := first("approve_digest")
	if operation == "" || pluginID == "" || version == "" || digest == "" {
		return nil
	}
	permissions := append([]string(nil), fields["approve_permission"]...)
	return []appplugins.PermissionApproval{{Operation: operation, PluginID: pluginID, Version: version, Digest: digest, Permissions: permissions}}
}
