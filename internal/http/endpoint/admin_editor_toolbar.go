package endpoint

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
)

// editorToolbarSettingsService owns persisted editor-toolbar overrides.
type editorToolbarSettingsService interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
	SaveEditorToolbarOverrides(context.Context, []domain.EditorToolbarOverride, int64) error
}

// AdminEditorToolbar renders global editor-toolbar configuration.
func AdminEditorToolbar(browserContext browserContextLoader, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Editor toolbar", "editor-toolbar")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		views.Render(w, "admin_editor_toolbar", webview.AdminEditorToolbarView{Layout: layout})
	}
}

// SaveAdminEditorToolbar validates and persists global editor-toolbar overrides.
func SaveAdminEditorToolbar(settingsUseCases editorToolbarSettingsService, manager *plugin.Manager, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid toolbar form.")
			return
		}

		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		overrides, err := editorToolbarOverridesFromForm(r, manager, settings.EditorToolbarOverrides)
		if err != nil {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Toolbar validation failed.", httpresponse.NewFieldProblem("editor_toolbar", err.Error()))
			return
		}

		if err := settingsUseCases.SaveEditorToolbarOverrides(r.Context(), overrides, currentUser(r).ID); err != nil {
			writeAdminProblem(views.Logger(), w, err, "Editor toolbar")
			return
		}

		http.Redirect(w, r, "/admin/editor-toolbar", http.StatusSeeOther)
	}
}

// editorToolbarOverridesFromForm merges active contribution changes while retaining stale overrides.
func editorToolbarOverridesFromForm(r *http.Request, manager *plugin.Manager, existing []domain.EditorToolbarOverride) ([]domain.EditorToolbarOverride, error) {
	known := map[string]plugin.ToolbarContribution{}
	for _, group := range manager.ResolveEditorToolbar(existing) {
		for _, contribution := range group.Contributions {
			known[contribution.ID] = contribution
		}
	}

	result := map[string]domain.EditorToolbarOverride{}
	for _, override := range existing {
		result[override.ID] = override
	}

	resetID := strings.TrimSpace(r.FormValue("toolbar_reset"))
	ids, groups, orders := r.Form["toolbar_id"], r.Form["toolbar_group"], r.Form["toolbar_order"]
	for index, id := range ids {
		contribution, ok := known[id]
		if !ok {
			return nil, fmt.Errorf("unknown contribution %q", id)
		}
		if id == resetID {
			delete(result, id)
			continue
		}

		group := formValueAt(groups, index)
		if !slices.Contains(contribution.AllowedGroups, group) {
			return nil, fmt.Errorf("group %q is not allowed for %s", group, contribution.Name)
		}

		order, err := strconv.Atoi(formValueAt(orders, index))
		if err != nil || order < -1000 || order > 1000 {
			return nil, fmt.Errorf("order for %s must be between -1000 and 1000", contribution.Name)
		}

		override := domain.EditorToolbarOverride{
			ID:     id,
			Group:  group,
			Hidden: r.FormValue("toolbar_visible_"+id) != "on",
			Order:  order,
		}
		if override.Group == contribution.DefaultGroup && !override.Hidden && override.Order == contribution.DefaultOrder {
			delete(result, id)
			continue
		}

		result[id] = override
	}

	values := make([]domain.EditorToolbarOverride, 0, len(result))
	for _, override := range result {
		values = append(values, override)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })

	return values, nil
}
