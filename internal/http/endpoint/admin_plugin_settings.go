package endpoint

import (
	"errors"
	"net/http"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// pluginSettingsAction identifies one mutation exposed by the plugin-settings route.
type pluginSettingsAction string

const (
	pluginSettingsActionSettings       pluginSettingsAction = "settings"
	pluginSettingsActionSettingsGroup  pluginSettingsAction = "settings-group"
	pluginSettingsActionResourceSave   pluginSettingsAction = "resource-save"
	pluginSettingsActionResourceDelete pluginSettingsAction = "resource-delete"
	pluginSettingsActionAdminAction    pluginSettingsAction = "admin-action"
)

// AdminPluginSettings renders and mutates configuration owned by installed plugins.
type AdminPluginSettings struct {
	// manager owns plugin settings and structured resource persistence.
	manager *plugin.Manager
	// data loads shared administration view data.
	data browserContextLoader
	// views renders plugin settings responses.
	views *webview.Views
}

// NewAdminPluginSettings constructs the dedicated plugin settings handler.
func NewAdminPluginSettings(manager *plugin.Manager, data browserContextLoader, views *webview.Views) *AdminPluginSettings {
	return &AdminPluginSettings{manager: manager, data: data, views: views}
}

// Show renders the settings page for one installed plugin.
func (a *AdminPluginSettings) Show(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, r.PathValue("pluginID"), http.StatusOK, "")
}

// Action applies one plugin-owned settings, resource, or administrator action.
func (a *AdminPluginSettings) Action(w http.ResponseWriter, r *http.Request) {
	pluginID := strings.TrimSpace(r.PathValue("pluginID"))
	action := pluginSettingsAction(strings.TrimSpace(r.PathValue("action")))

	var err error
	switch action {
	case pluginSettingsActionSettings:
		err = a.updateSettings(r, pluginID)
	case pluginSettingsActionSettingsGroup:
		err = a.saveSettingsGroup(r, pluginID)
	case pluginSettingsActionResourceSave:
		err = a.saveResource(r, pluginID)
	case pluginSettingsActionResourceDelete:
		err = a.deleteResource(r, pluginID)
	case pluginSettingsActionAdminAction:
		err = a.runAdminAction(r, pluginID)
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.writeActionError(w, r, pluginID, action, err)
		return
	}

	if action == pluginSettingsActionAdminAction {
		a.views.Logger().Info(
			"plugin admin action completed",
			"event", "plugin.admin_action_completed",
			"plugin_id", pluginID,
			"module_id", strings.TrimSpace(r.FormValue("action_id")),
			"actor_id", currentUser(r).ID,
		)
	} else {
		a.views.Logger().Info(
			"plugin settings changed",
			"event", "plugin.settings_changed",
			"plugin_id", pluginID,
			"action", action,
			"actor_id", currentUser(r).ID,
		)
	}
	http.Redirect(w, r, "/admin/plugin-settings/"+pluginID, http.StatusSeeOther)
}

// writeActionError translates expected plugin-setting failures and logs unexpected ones.
func (a *AdminPluginSettings) writeActionError(w http.ResponseWriter, r *http.Request, pluginID string, action pluginSettingsAction, err error) {
	message := "Could not save plugin settings. Check the entered values and setting dependencies."
	if action == pluginSettingsActionAdminAction {
		message = "Could not run the plugin action."
	}
	problems := []httpresponse.FieldProblem(nil)
	expected := false

	if fieldErr, ok := errors.AsType[*plugin.ConfigurationFieldError](err); ok {
		message = "Plugin resource validation failed."
		fieldName := "resource_" + fieldErr.Field
		if action == pluginSettingsActionSettingsGroup {
			message = "Plugin settings validation failed."
			fieldName = "setting_" + strings.TrimSpace(r.FormValue("settings_id")) + "_" + fieldErr.Field
		}
		problems = append(problems, httpresponse.NewFieldProblem(fieldName, fieldErr.Message))
		expected = true
	} else if errors.Is(err, plugin.ErrSecretEncryptionUnavailable) {
		message = "Configure KUMBUKA__ENCRYPTION_KEY before saving plugin secrets."
		expected = true
	}

	if !expected {
		if action == pluginSettingsActionAdminAction {
			a.views.Logger().Error(
				"plugin admin action failed",
				"event", "plugin.admin_action_failed",
				"plugin_id", pluginID,
				"module_id", strings.TrimSpace(r.FormValue("action_id")),
				"error", err,
				"actor_id", currentUser(r).ID,
			)
		} else {
			a.views.Logger().Error(
				"plugin settings update failed",
				"event", "plugin.settings_update_failed",
				"plugin_id", pluginID,
				"action", action,
				"error", err,
				"actor_id", currentUser(r).ID,
			)
		}
	} else if errors.Is(err, plugin.ErrSecretEncryptionUnavailable) {
		a.views.Logger().Warn(
			"plugin settings require application encryption",
			"event", "plugin.settings_encryption_required",
			"plugin_id", pluginID,
			"action", action,
			"actor_id", currentUser(r).ID,
		)
	}

	if wantsJSON(r) {
		httpresponse.Problem(w, http.StatusUnprocessableEntity, message, problems...)
		return
	}

	a.render(w, r, pluginID, http.StatusUnprocessableEntity, message)
}

// render loads one plugin's settings and structured resources into the administration layout.
func (a *AdminPluginSettings) render(w http.ResponseWriter, r *http.Request, pluginID string, status int, message string) {
	w.Header().Set("Cache-Control", "private, no-store")
	if a.manager == nil {
		http.Error(w, "Plugin manager unavailable.", http.StatusServiceUnavailable)
		return
	}

	selected, ok := loadedPlugin(a.manager, pluginID)
	if !ok || !pluginHasAdminSettings(selected) {
		http.NotFound(w, r)
		return
	}
	layout, err := administrationData(r, a.data, a.views, selected.Manifest.Name+" settings", "plugin:"+selected.Manifest.ID)
	data := webview.AdminPluginSettingsView{Layout: layout}
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}

	settingGroups, err := a.manager.SettingGroups(r.Context(), selected.Manifest.ID)
	if err != nil {
		httpresponse.InternalServerError(a.views.Logger(), w, err)
		return
	}

	resources := make([]webview.PluginResource, 0)
	for _, module := range selected.Manifest.Modules {
		if plugin.ModuleType(module.Type) != plugin.ModuleTypeAdminResource {
			continue
		}
		records, resourceErr := a.manager.ResourceRecords(r.Context(), selected.Manifest.ID, module.ID)
		if resourceErr != nil {
			httpresponse.InternalServerError(a.views.Logger(), w, resourceErr)
			return
		}
		resources = append(resources, webview.PluginResource{Module: module, Records: records})
	}

	data.PluginSettings = &selected
	data.PluginSettingsGroups = settingGroups
	data.PluginSettingsResources = resources
	data.PluginMessage = message
	a.views.RenderStatus(w, status, "admin_plugin_settings", data)
}

// updateSettings persists every declared boolean setting for one plugin.
func (a *AdminPluginSettings) updateSettings(r *http.Request, pluginID string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	selected, ok := loadedPlugin(a.manager, pluginID)
	if !ok {
		return errors.New("plugin is not installed")
	}

	settings := make(map[string]bool)
	for _, module := range selected.Manifest.Modules {
		if plugin.ModuleType(module.Type) == plugin.ModuleTypeSettings && len(module.Fields) == 0 {
			settings[module.ID] = r.Form.Has("setting_" + module.ID)
		}
	}
	return a.manager.UpdateSettings(r.Context(), pluginID, settings)
}

// saveSettingsGroup validates and persists one typed singleton plugin settings group.
func (a *AdminPluginSettings) saveSettingsGroup(r *http.Request, pluginID string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	selected, ok := loadedPlugin(a.manager, pluginID)
	if !ok {
		return errors.New("plugin is not installed")
	}
	module, ok := pluginSettingsGroupModule(selected, strings.TrimSpace(r.FormValue("settings_id")))
	if !ok {
		return errors.New("plugin settings group is not declared")
	}

	values := make(map[string]string, len(module.Fields))
	for _, field := range module.Fields {
		name := "setting_" + module.ID + "_" + field.ID
		if plugin.ConfigurationFieldType(field.Type) == plugin.ConfigurationFieldBoolean {
			values[field.ID] = "false"
			if r.Form.Has(name) {
				values[field.ID] = "true"
			}
			continue
		}
		values[field.ID] = r.FormValue(name)
	}

	return a.manager.SaveSettingGroup(r.Context(), pluginID, module.ID, values)
}

// saveResource validates and persists one structured plugin setting record.
func (a *AdminPluginSettings) saveResource(r *http.Request, pluginID string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	selected, ok := loadedPlugin(a.manager, pluginID)
	if !ok {
		return errors.New("plugin is not installed")
	}
	module, ok := pluginResourceModule(selected, strings.TrimSpace(r.FormValue("resource_id")))
	if !ok {
		return errors.New("plugin resource is not declared")
	}

	values := make(map[string]string, len(module.Fields))
	for _, field := range module.Fields {
		name := "resource_" + field.ID
		if plugin.ConfigurationFieldType(field.Type) == plugin.ConfigurationFieldBoolean {
			if r.Form.Has(name) {
				values[field.ID] = "true"
			} else {
				values[field.ID] = "false"
			}
			continue
		}
		values[field.ID] = r.FormValue(name)
	}
	return a.manager.SaveResourceRecord(r.Context(), pluginID, module.ID, r.FormValue("original_key"), values)
}

// deleteResource removes one structured plugin setting record.
func (a *AdminPluginSettings) deleteResource(r *http.Request, pluginID string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	return a.manager.DeleteResourceRecord(r.Context(), pluginID, strings.TrimSpace(r.FormValue("resource_id")), r.FormValue("record_key"))
}

// runAdminAction invokes one explicitly declared executable administration action.
func (a *AdminPluginSettings) runAdminAction(r *http.Request, pluginID string) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	selected, ok := loadedPlugin(a.manager, pluginID)
	if !ok {
		return errors.New("plugin is not installed")
	}
	actionID := strings.TrimSpace(r.FormValue("action_id"))
	if !pluginAdminActionModule(selected, actionID) {
		return errors.New("plugin admin action is not declared")
	}
	return a.manager.RunAdminAction(r.Context(), pluginID, actionID)
}

// loadedPlugin returns one installed plugin by stable ID.
func loadedPlugin(manager *plugin.Manager, pluginID string) (plugin.LoadedPlugin, bool) {
	for _, item := range manager.Plugins() {
		if item.Manifest.ID == pluginID {
			return item, true
		}
	}
	return plugin.LoadedPlugin{}, false
}

// pluginHasAdminSettings reports whether a plugin exposes settings, structured resources, or administrator actions.
func pluginHasAdminSettings(item plugin.LoadedPlugin) bool {
	for _, module := range item.Manifest.Modules {
		if isPluginAdminModule(module) {
			return true
		}
	}
	return false
}

// isPluginAdminModule reports whether a plugin module contributes administrator-managed functionality.
func isPluginAdminModule(module pluginpackage.Module) bool {
	switch plugin.ModuleType(module.Type) {
	case plugin.ModuleTypeSettings, plugin.ModuleTypeAdminResource, plugin.ModuleTypeAdminAction:
		return true
	default:
		return false
	}
}

// isPluginSettingsGroup reports whether module is the requested non-empty typed settings group.
func isPluginSettingsGroup(module pluginpackage.Module, moduleID string) bool {
	return plugin.ModuleType(module.Type) == plugin.ModuleTypeSettings && len(module.Fields) != 0 && module.ID == moduleID
}

// pluginSettingsGroupModule returns one declared typed settings group from a loaded plugin.
func pluginSettingsGroupModule(item plugin.LoadedPlugin, moduleID string) (pluginpackage.Module, bool) {
	for _, module := range item.Manifest.Modules {
		if isPluginSettingsGroup(module, moduleID) {
			return module, true
		}
	}
	return pluginpackage.Module{}, false
}

// pluginResourceModule returns one declared structured resource from a loaded plugin.
func pluginResourceModule(item plugin.LoadedPlugin, moduleID string) (pluginpackage.Module, bool) {
	for _, module := range item.Manifest.Modules {
		if plugin.ModuleType(module.Type) == plugin.ModuleTypeAdminResource && module.ID == moduleID {
			return module, true
		}
	}
	return pluginpackage.Module{}, false
}

// pluginAdminActionModule reports whether one executable administrator action is declared by a plugin.
func pluginAdminActionModule(item plugin.LoadedPlugin, moduleID string) bool {
	for _, module := range item.Manifest.Modules {
		if plugin.ModuleType(module.Type) == plugin.ModuleTypeAdminAction && module.ID == moduleID {
			return true
		}
	}
	return false
}
