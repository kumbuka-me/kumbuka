package handler

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
)

// AdminNavigation renders icon configuration for every navigation path.
func AdminNavigation(
	viewDataUseCases viewDataService,
	navigationUseCases navigationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Navigation", "navigation")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		items, err := navigationUseCases.NavigationItems(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminNavigation = items

		views.Render(w, "admin_navigation", data)
	}
}

// SearchIcons serves icon picker results to page editors and administrators.
func SearchIcons(catalog *icons.Catalog) http.HandlerFunc {
	if catalog == nil {
		catalog = icons.Builtin()
	}
	type result struct {
		Name   string `json:"name"`
		Label  string `json:"label"`
		Source string `json:"source"`
		SVG    string `json:"svg"`
	}
	type response struct {
		Items   []result `json:"items"`
		HasMore bool     `json:"has_more"`
	}

	return func(w http.ResponseWriter, r *http.Request) {
		offset := 0

		if value := r.URL.Query().Get("offset"); value != "" {
			var err error
			offset, err = strconv.Atoi(value)
			if err != nil || offset < 0 {
				httpresponse.Problem(w, http.StatusBadRequest, "Icon search offset must be a non-negative integer.")
				return
			}
		}

		options, hasMore := catalog.SearchPage(r.URL.Query().Get("q"), offset, 80)
		results := make([]result, 0, len(options))

		for _, option := range options {
			results = append(
				results,
				result{
					Name:   option.Name,
					Label:  option.Label,
					Source: option.Source,
					SVG:    string(catalog.SVG(option.Name, 22)),
				},
			)
		}

		httpresponse.Respond(w, http.StatusOK, response{Items: results, HasMore: hasMore})
	}
}

// SaveAdminNavigationIcon stores the selected icon for one navigation path.
func SaveAdminNavigationIcon(navigationUseCases navigationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid navigation form.")
			return
		}

		path := strings.TrimSpace(r.FormValue("path"))
		icon := strings.TrimSpace(r.FormValue("icon"))
		if err := navigationUseCases.SetNavigationIcon(r.Context(), path, icon); err != nil {
			writeAdminProblem(logger, w, err, "Navigation path")
			return
		}

		http.Redirect(w, r, "/admin/navigation", http.StatusSeeOther)
	}
}

// AdminTags renders tag management and usage counts.
func AdminTags(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Tags", "tags")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		tags, err := administrationUseCases.TagInfos(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminTags = tags

		views.Render(w, "admin_tags", data)
	}
}

// DeleteAdminTag removes a tag and all page associations for it.
func DeleteAdminTag(administrationUseCases administrationService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid tag identifier.")
			return
		}
		if err := administrationUseCases.DeleteTag(r.Context(), id); err != nil {
			writeAdminProblem(logger, w, err, "Tag")
			return
		}

		http.Redirect(w, r, "/admin/tags", http.StatusSeeOther)
	}
}
