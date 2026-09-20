package endpoint

import (
	"net/http"
	"strings"
	"time"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// AdminDocumentationHealth renders actionable wiki documentation-quality findings.
func AdminDocumentationHealth(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Documentation health", "health")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		health, err := administrationUseCases.DocumentationHealth(r.Context(), time.Now().AddDate(0, -6, 0))
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.DocumentationHealth = health

		views.Render(w, "admin_health", data)
	}
}

// AdminAudit renders recent application audit events.
func AdminAudit(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Audit log", "audit")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		events, err := administrationUseCases.AuditEvents(r.Context(), 500)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AuditEvents = events

		views.Render(w, "admin_audit", data)
	}
}

// AdminTokens renders administrator-managed personal access tokens.
func AdminTokens(
	viewDataUseCases viewDataService,
	userUseCases userManagementService,
	tokenUseCases tokenService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Access tokens", "tokens")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		users, err := userUseCases.Users(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		tokens, err := tokenUseCases.Tokens(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminUsers = users
		data.AdminTokens = tokens

		views.Render(w, "admin_tokens", data)
	}
}

// AdminExports renders page export controls.
func AdminExports(
	viewDataUseCases viewDataService,
	navigationUseCases navigationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Exports", "exports")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		pages, err := navigationUseCases.NavigationPages(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminPages = pages

		views.Render(w, "admin_exports", data)
	}
}

// AdminImages renders all uploaded images and their reference counts.
func AdminImages(
	viewDataUseCases viewDataService,
	mediaUseCases imageListService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Images", "images")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.ImageQuery = strings.TrimSpace(r.URL.Query().Get("image_q"))
		images, err := mediaUseCases.SearchImages(
			r.Context(),
			data.ImageQuery,
			managedImagePageSize+1,
			0,
		)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Images, data.ImagesHasMore = managedImageItems(images)

		views.Render(w, "admin_images", data)
	}
}
