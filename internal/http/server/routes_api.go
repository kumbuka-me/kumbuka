package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// registerAPIRoutes registers authenticated machine-readable application endpoints.
func registerAPIRoutes(mux *http.ServeMux, config Config) {
	apiAuthn := middleware.AuthenticateAPI(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator)
	adminAuthz := middleware.RequireRole(domain.UserRoleAdmin)
	editorAuthz := middleware.RequireRole(domain.UserRoleAdmin, domain.UserRoleEditor)

	mux.Handle("GET /api/icons", apiAuthn(editorAuthz(endpoint.SearchIcons(config.Views.IconCatalog()))))
	mux.Handle("GET /api/pages", apiAuthn(endpoint.ListPages(config.PageSearch, config.Logger)))
	mux.Handle("POST /api/pages", apiAuthn(editorAuthz(endpoint.SavePage(config.PageMutations, config.Logger))))
	mux.Handle(
		"POST /api/preview",
		apiAuthn(editorAuthz(endpoint.PreviewMarkdown(
			config.Navigation,
			config.PageReports,
			config.Renderer,
			config.Logger,
		))),
	)
	mux.Handle("GET /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.GetPageDraft(config.Drafts, config.Logger))))
	mux.Handle("PUT /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.SavePageDraft(config.Drafts, config.Logger))))
	mux.Handle("DELETE /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.DeletePageDraft(config.Drafts, config.Logger))))
	mux.Handle("GET /api/page-presence/{slug...}", apiAuthn(endpoint.PageEditors(config.PagePresence, config.Logger)))
	mux.Handle("PUT /api/page-presence/{slug...}", apiAuthn(editorAuthz(endpoint.TouchPageEditor(config.PagePresence, config.Logger))))
	mux.Handle("DELETE /api/page-presence/{slug...}", apiAuthn(editorAuthz(endpoint.LeavePageEditor(config.PagePresence, config.Logger))))
	mux.Handle("GET /api/pages/{slug...}", apiAuthn(endpoint.GetPage(config.PageLookup, config.Logger)))
	mux.Handle("PUT /api/pages/{slug...}", apiAuthn(editorAuthz(endpoint.SavePage(config.PageMutations, config.Logger))))
	mux.Handle("DELETE /api/pages/{slug...}", apiAuthn(adminAuthz(endpoint.DeletePage(config.PageMutations, config.Logger))))
	mux.Handle("GET /api/search", apiAuthn(endpoint.SearchAPI(config.PageSearch, config.Logger)))
	mux.Handle("GET /api/graph", apiAuthn(endpoint.KnowledgeGraphAPI(config.Knowledge, config.Logger)))
	mux.Handle(
		"GET /api/editor/catalog",
		apiAuthn(editorAuthz(endpoint.EditorCatalog(config.Navigation, config.PageDirectory, config.Settings, config.Renderer.PluginManager(), config.Logger))),
	)
	mux.Handle("GET /api/mentions/users", apiAuthn(endpoint.MentionUsers(config.Users, config.Logger)))
	mux.Handle("GET /api/notifications", apiAuthn(endpoint.NotificationsAPI(config.Notifications, config.Logger)))
	mux.Handle("POST /api/notifications/{id}/read", apiAuthn(endpoint.MarkNotificationRead(config.Notifications, config.Logger)))
	mux.Handle("POST /api/notifications/{id}/unread", apiAuthn(endpoint.MarkNotificationUnread(config.Notifications, config.Logger)))
	mux.Handle("DELETE /api/notifications/{id}", apiAuthn(endpoint.DeleteNotification(config.Notifications, config.Logger)))
	mux.Handle("GET /api/tags", apiAuthn(endpoint.Tags(config.PageSearch, config.Logger)))
	mux.Handle("GET /api/groups", apiAuthn(endpoint.GroupsAPI(config.Groups, config.Logger)))
	mux.Handle("GET /api/images", apiAuthn(editorAuthz(endpoint.ListImages(config.Media, config.Logger))))
	mux.Handle("GET /api/attachments", apiAuthn(editorAuthz(endpoint.ListAttachments(config.Media, config.Logger))))
	mux.Handle("POST /api/attachments", apiAuthn(editorAuthz(endpoint.UploadAttachment(config.Media, config.Logger))))
	mux.Handle("DELETE /api/attachments/{id}", apiAuthn(editorAuthz(endpoint.DeleteAttachment(config.Media, config.Logger))))
	mux.Handle("POST /api/images", apiAuthn(editorAuthz(endpoint.UploadImage(config.Media, config.Logger))))
	mux.Handle("DELETE /api/images/{id}", apiAuthn(editorAuthz(endpoint.DeleteImage(config.Media, config.Logger))))
	mux.Handle("GET /api/recent", apiAuthn(endpoint.Recent(config.PageSearch, config.Logger)))
}
