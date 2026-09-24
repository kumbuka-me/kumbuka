package endpoint

import (
	"log/slog"
	"net/http"
	"strconv"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// AdminPageAccess renders inherited page-path access rules.
func AdminPageAccess(
	browserContext browserContextLoader,
	accessUseCases pageAccessAdmin,
	groupUseCases groupReader,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Page access", "permissions")
		data := webview.AdminPermissionsView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.PageAccessRules, err = accessUseCases.PageAccessRules(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		data.Groups, err = groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		views.Render(w, "admin_permissions", data)
	}
}

// SaveAdminPageAccess creates or replaces one inherited path rule.
func SaveAdminPageAccess(accessUseCases pageAccessAdmin, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid access form.")
			return
		}
		groupID, err := strconv.ParseInt(r.FormValue("group_id"), 10, 64)
		if err != nil {
			httpresponse.Problem(w, http.StatusUnprocessableEntity, "Page access validation failed.", httpresponse.NewFieldProblem("group_id", "Choose a group."))
			return
		}
		if err := accessUseCases.SavePageAccessRule(r.Context(), r.FormValue("path"), groupID, domain.PageAccessLevel(r.FormValue("access"))); err != nil {
			writeAdminProblem(logger, w, err, "Page access rule")
			return
		}
		http.Redirect(w, r, "/admin/permissions", http.StatusSeeOther)
	}
}

// DeleteAdminPageAccess removes one inherited path rule.
func DeleteAdminPageAccess(accessUseCases pageAccessAdmin, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid access rule identifier.")
			return
		}
		if err := accessUseCases.DeletePageAccessRule(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Page access rule")
			return
		}
		http.Redirect(w, r, "/admin/permissions", http.StatusSeeOther)
	}
}
