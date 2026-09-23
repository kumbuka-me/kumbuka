package endpoint

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	apptemplates "github.com/kumbuka-me/kumbuka/internal/application/templates"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// AdminPageTemplates renders reusable page-template management.
func AdminPageTemplates(
	browserContext browserContextLoader,
	templateUseCases templateService,
	groupUseCases groupReader,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Page templates", "templates")
		data := webview.AdminTemplatesView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		templates, err := templateUseCases.PageTemplates(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		groups, err := groupUseCases.Groups(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.PageTemplates = templates
		data.Groups = groups
		data.PageStatuses = domain.PageStatuses()

		views.Render(w, "admin_templates", data)
	}
}

// CreateAdminPageTemplate creates a reusable Markdown page template.
func CreateAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		input, err := pageTemplateInputFromForm(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		if _, err := templateUseCases.CreatePageTemplate(r.Context(), input); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}

// UpdateAdminPageTemplate updates one reusable Markdown page template.
func UpdateAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template identifier.")
			return
		}
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		input, err := pageTemplateInputFromForm(r)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template form.")
			return
		}
		if err := templateUseCases.UpdatePageTemplate(r.Context(), id, input); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}

// pageTemplateInputFromForm translates the blueprint form into a service input without coercing malformed numbers.
func pageTemplateInputFromForm(r *http.Request) (apptemplates.PageTemplateInput, error) {
	ownerGroupID, err := parseOptionalFormInt64(r.FormValue("owner_group_id"))
	if err != nil {
		return apptemplates.PageTemplateInput{}, err
	}

	reviewIntervalDays, err := parseOptionalFormInt(r.FormValue("review_interval_days"))
	if err != nil {
		return apptemplates.PageTemplateInput{}, err
	}

	return apptemplates.PageTemplateInput{
		Name:               r.FormValue("name"),
		Description:        r.FormValue("description"),
		Markdown:           r.FormValue("markdown"),
		PathPrefix:         r.FormValue("path_prefix"),
		Icon:               r.FormValue("icon"),
		Tags:               splitTags(r.FormValue("tags")),
		Status:             r.FormValue("status"),
		OwnerGroupID:       ownerGroupID,
		ReviewIntervalDays: reviewIntervalDays,
		Properties:         parseBlueprintProperties(r.FormValue("properties")),
		Fields:             parseBlueprintFields(r.FormValue("fields")),
	}, nil
}

// parseBlueprintProperties parses one key=value blueprint property per line.
func parseBlueprintProperties(value string) map[string]string {
	properties := map[string]string{}

	for line := range strings.SplitSeq(value, "\n") {
		key, content, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(key)

		if !ok || key == "" {
			continue
		}

		properties[key] = strings.TrimSpace(content)
	}

	return properties
}

// parseBlueprintFields parses the compact blueprint field definition format.
func parseBlueprintFields(value string) []domain.PageTemplateField {
	fields := make([]domain.PageTemplateField, 0)

	for line := range strings.SplitSeq(value, "\n") {
		field, ok := parseBlueprintField(line)
		if !ok {
			continue
		}

		fields = append(fields, field)
	}

	return fields
}

// parseBlueprintField parses one name|label|default|required blueprint field line.
func parseBlueprintField(value string) (domain.PageTemplateField, bool) {
	parts := strings.Split(value, "|")
	name := formValueAt(parts, 0)
	if name == "" {
		return domain.PageTemplateField{}, false
	}

	return domain.PageTemplateField{
		Name:     name,
		Label:    formValueAt(parts, 1),
		Default:  formValueAt(parts, 2),
		Required: strings.EqualFold(formValueAt(parts, 3), "required"),
	}, true
}

// DeleteAdminPageTemplate deletes one reusable page template.
func DeleteAdminPageTemplate(templateUseCases templateService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid template identifier.")
			return
		}
		if err := templateUseCases.DeletePageTemplate(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Page template")
			return
		}

		http.Redirect(w, r, "/admin/templates", http.StatusSeeOther)
	}
}
