package endpoint

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
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
		layout, err := administrationData(r, viewDataUseCases, views, "Navigation", "navigation")
		data := webview.AdminNavigationView{Layout: layout}
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
	// result is one icon picker option, including its rendered preview.
	type result struct {
		// Name is the catalog identifier persisted when this icon is selected.
		Name string `json:"name"`
		// Label is the human-readable icon name.
		Label string `json:"label"`
		// Source identifies the built-in or plugin icon collection.
		Source string `json:"source"`
		// SVG contains the catalog-rendered preview markup.
		SVG string `json:"svg"`
	}
	// response contains one page of icon search results.
	type response struct {
		// Items contains the matching icons in display order.
		Items []result `json:"items"`
		// HasMore tells the picker whether another page is available.
		HasMore bool `json:"has_more"`
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
		layout, err := administrationData(r, viewDataUseCases, views, "Tags", "tags")
		data := webview.AdminTagsView{Layout: layout}
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
