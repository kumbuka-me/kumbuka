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
	browserContext browserContextLoader,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Documentation health", "health")
		data := webview.AdminHealthView{Layout: layout}
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
	browserContext browserContextLoader,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Audit log", "audit")
		data := webview.AdminAuditView{Layout: layout}
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
	browserContext browserContextLoader,
	userUseCases userManagementService,
	tokenUseCases tokenService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Access tokens", "tokens")
		data := webview.AdminTokensView{Layout: layout}
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
	browserContext browserContextLoader,
	navigationUseCases navigationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Exports", "exports")
		data := webview.AdminExportsView{Layout: layout}
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
	browserContext browserContextLoader,
	mediaUseCases imageListService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		layout, err := administrationData(r, browserContext, views, "Images", "images")
		data := webview.AdminImagesView{Layout: layout}
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
