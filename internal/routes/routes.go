package routes

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// addRoutes registers the complete HTTP surface and applies route-specific access policies.
func addRoutes(
	mux *http.ServeMux,
	appFS fs.FS,
	views *handler.Views,
	renderer *markdown.Renderer,
	browserAuth auth.BrowserAuth,
	administrationUseCases *service.Administration,
	accessUseCases *service.Access,
	catalogUseCases *service.Catalog,
	draftUseCases *service.Drafts,
	groupUseCases *service.Groups,
	knowledgeUseCases *service.Knowledge,
	notificationUseCases *service.Notifications,
	mediaUseCases *service.Media,
	navigationUseCases *service.Navigation,
	pageUseCases *service.Pages,
	preferenceUseCases *service.Preferences,
	recycleBinUseCases *service.RecycleBin,
	settingsUseCases *service.Settings,
	systemUseCases *service.System,
	templateUseCases *service.Templates,
	tokenUseCases *service.Tokens,
	userUseCases *service.Users,
	webhookUseCases *service.Webhooks,
	viewDataUseCases *handler.ViewDataLoader,
	logger *slog.Logger,
	browserAuthn middleware.Middleware,
	mediaAuthn middleware.Middleware,
	apiAuthn middleware.Middleware,
	adminAuthz middleware.Middleware,
	editorAuthz middleware.Middleware,
) {
	pageViewAuthz := middleware.RequirePageView(accessUseCases)
	pageEditAuthz := middleware.RequirePageEdit(accessUseCases)

	pluginsAdmin := handler.NewAdminPlugins(renderer.PluginManager(), viewDataUseCases, views)
	mux.Handle("GET /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.List))))
	mux.Handle("POST /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Install))))
	mux.Handle("POST /admin/plugins/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Action))))
	// Public infrastructure and authentication routes.
	mux.HandleFunc("GET /plugins/styles.css", handler.PluginPresentationStyles(renderer.PluginManager()))
	mux.HandleFunc("GET /plugins/runtime.js", handler.PluginBrowserRuntime(appFS))
	mux.HandleFunc("GET /plugins/{pluginID}/{digest}/assets/{asset...}", handler.PluginAssets(renderer.PluginManager()))
	mux.HandleFunc("GET /plugins/{pluginID}/{digest}/frames/{frame}", handler.PluginFrame(renderer.PluginManager()))
	mux.HandleFunc("GET /healthz", handler.Health(systemUseCases))
	mux.Handle("GET /robots.txt", handler.Robots(settingsUseCases, views, logger))
	mux.Handle("GET /sitemap.xml", handler.Sitemap(settingsUseCases, catalogUseCases, accessUseCases, views, logger))
	mux.Handle("GET /assets/", handler.Assets(appFS))
	mux.Handle("GET /sw.js", handler.ServiceWorker(appFS))
	mux.Handle("GET /auth/login", browserAuth.Login)
	mux.Handle("GET /auth/local", handler.LocalLogin(settingsUseCases, systemUseCases, browserAuth, views))
	mux.Handle("POST /auth/local", handler.LocalLogin(settingsUseCases, systemUseCases, browserAuth, views))
	mux.Handle("GET /setup", handler.Setup(settingsUseCases, systemUseCases, browserAuth, views))
	mux.Handle("POST /setup", handler.Setup(settingsUseCases, systemUseCases, browserAuth, views))

	if browserAuth.Callback != nil {
		mux.Handle("GET /auth/callback", browserAuth.Callback)
	}

	// Browser routes require the configured browser authenticator.
	mux.Handle("POST /auth/logout", browserAuthn(auth.Logout(browserAuth.Local)))
	mux.Handle("GET /{$}", browserAuthn(handler.Home(viewDataUseCases, catalogUseCases, draftUseCases, accessUseCases, renderer, views)))
	mux.Handle("GET /search", browserAuthn(handler.Search(viewDataUseCases, catalogUseCases, accessUseCases, views)))
	mux.Handle("GET /graph", browserAuthn(handler.KnowledgeGraphPage(viewDataUseCases, views)))
	mux.Handle("GET /p/{id}", browserAuthn(handler.PagePermalink(catalogUseCases, logger)))
	mux.Handle(
		"GET /settings",
		browserAuthn(handler.Settings(viewDataUseCases, userUseCases, tokenUseCases, mediaUseCases, browserAuth.Local, views)),
	)
	mux.Handle(
		"GET /admin",
		browserAuthn(adminAuthz(handler.Administration(viewDataUseCases, administrationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/configuration",
		browserAuthn(adminAuthz(handler.AdminConfiguration(viewDataUseCases, groupUseCases, userUseCases, settingsUseCases, views))),
	)
	mux.Handle(
		"GET /admin/health",
		browserAuthn(adminAuthz(handler.AdminDocumentationHealth(viewDataUseCases, administrationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/templates",
		browserAuthn(adminAuthz(handler.AdminPageTemplates(viewDataUseCases, templateUseCases, groupUseCases, views))),
	)
	mux.Handle("GET /admin/permissions", browserAuthn(adminAuthz(handler.AdminPageAccess(viewDataUseCases, accessUseCases, groupUseCases, views))))
	mux.Handle("GET /admin/webhooks", browserAuthn(adminAuthz(handler.AdminWebhooks(viewDataUseCases, webhookUseCases, views))))
	mux.Handle("POST /admin/webhooks", browserAuthn(adminAuthz(handler.SaveAdminWebhook(webhookUseCases, logger))))
	mux.Handle("POST /admin/webhooks/{id}", browserAuthn(adminAuthz(handler.SaveAdminWebhook(webhookUseCases, logger))))
	mux.Handle("POST /admin/webhooks/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminWebhook(webhookUseCases, logger))))
	mux.Handle("POST /admin/webhooks/{id}/test", browserAuthn(adminAuthz(handler.TestAdminWebhook(webhookUseCases, logger))))
	mux.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", browserAuthn(adminAuthz(handler.RevealAdminWebhookHeader(webhookUseCases, logger))))
	mux.Handle("POST /admin/permissions", browserAuthn(adminAuthz(handler.SaveAdminPageAccess(accessUseCases, logger))))
	mux.Handle("POST /admin/permissions/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminPageAccess(accessUseCases, logger))))
	mux.Handle(
		"GET /admin/audit",
		browserAuthn(adminAuthz(handler.AdminAudit(viewDataUseCases, administrationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/pages",
		browserAuthn(adminAuthz(handler.AdminPages(viewDataUseCases, catalogUseCases, groupUseCases, views))),
	)
	mux.Handle(
		"POST /admin/pages/bulk",
		browserAuthn(adminAuthz(handler.BulkAdminPages(pageUseCases, catalogUseCases, mediaUseCases, logger))),
	)
	mux.Handle("GET /admin/import", browserAuthn(adminAuthz(handler.AdminImport(viewDataUseCases, views))))
	mux.Handle("POST /admin/import", browserAuthn(adminAuthz(handler.ImportPages(pageUseCases, logger))))
	mux.Handle(
		"POST /admin/templates",
		browserAuthn(adminAuthz(handler.CreateAdminPageTemplate(templateUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/templates/{id}",
		browserAuthn(adminAuthz(handler.UpdateAdminPageTemplate(templateUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/templates/{id}/delete",
		browserAuthn(adminAuthz(handler.DeleteAdminPageTemplate(templateUseCases, logger))),
	)
	mux.Handle(
		"GET /admin/users",
		browserAuthn(adminAuthz(handler.AdminUsers(viewDataUseCases, userUseCases, groupUseCases, views))),
	)
	mux.Handle(
		"GET /admin/groups",
		browserAuthn(adminAuthz(handler.AdminGroups(viewDataUseCases, groupUseCases, views))),
	)
	mux.Handle(
		"GET /admin/navigation",
		browserAuthn(adminAuthz(handler.AdminNavigation(viewDataUseCases, navigationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/bin",
		browserAuthn(adminAuthz(handler.AdminBin(viewDataUseCases, recycleBinUseCases, views))),
	)
	mux.Handle(
		"GET /admin/tags",
		browserAuthn(adminAuthz(handler.AdminTags(viewDataUseCases, administrationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/tokens",
		browserAuthn(adminAuthz(handler.AdminTokens(viewDataUseCases, userUseCases, tokenUseCases, views))),
	)
	mux.Handle(
		"GET /admin/exports",
		browserAuthn(adminAuthz(handler.AdminExports(viewDataUseCases, navigationUseCases, views))),
	)
	mux.Handle(
		"GET /admin/images",
		browserAuthn(adminAuthz(handler.AdminImages(viewDataUseCases, mediaUseCases, views))),
	)
	mux.Handle("POST /admin/settings", browserAuthn(adminAuthz(handler.SaveAdminSettings(settingsUseCases, logger))))
	mux.Handle("POST /admin/pdf", browserAuthn(adminAuthz(handler.SaveAdminPDFSettings(settingsUseCases, logger))))
	mux.Handle("POST /admin/pdf/test", browserAuthn(adminAuthz(handler.TestAdminPDFService(settingsUseCases, logger))))
	mux.Handle("POST /admin/pdf/headers/{id}/reveal", browserAuthn(adminAuthz(handler.RevealAdminPDFHeader(settingsUseCases, logger))))
	mux.Handle(
		"POST /admin/authentication",
		browserAuthn(adminAuthz(handler.SaveAdminAuthentication(settingsUseCases, browserAuth, views))),
	)

	mux.Handle("GET /media/{id}/{name...}", mediaAuthn(handler.ServeImage(mediaUseCases, logger)))
	mux.Handle("GET /attachments/{id}/{name...}", mediaAuthn(handler.ServeAttachment(mediaUseCases, logger)))
	mux.Handle("POST /settings/preferences", browserAuthn(handler.SavePreferences(preferenceUseCases, renderer.PluginManager(), views)))
	mux.Handle("POST /settings/local-password", browserAuthn(handler.ChangeLocalPassword(browserAuth.Local, logger)))
	mux.Handle(
		"POST /settings/preferences/page-contents",
		browserAuthn(handler.SavePageContentsPreference(preferenceUseCases, views)),
	)
	mux.Handle(
		"POST /settings/preferences/navigation-state",
		browserAuthn(handler.SaveNavigationState(preferenceUseCases, logger)),
	)
	mux.Handle(
		"POST /settings/preferences/sidebar-width",
		browserAuthn(handler.SaveSidebarWidth(preferenceUseCases, logger)),
	)
	mux.Handle("POST /settings/saved-searches", browserAuthn(handler.CreateSavedSearch(knowledgeUseCases, logger)))
	mux.Handle(
		"POST /settings/saved-searches/{id}/delete",
		browserAuthn(handler.DeleteSavedSearch(knowledgeUseCases, logger)),
	)
	mux.Handle("POST /settings/tokens", browserAuthn(handler.CreatePersonalToken(tokenUseCases, logger)))
	mux.Handle("DELETE /settings/tokens/{id}", browserAuthn(handler.DeletePersonalToken(tokenUseCases, logger)))
	mux.Handle(
		"POST /admin/users/{id}",
		browserAuthn(adminAuthz(handler.UpdateAdminUser(userUseCases, views, logger))),
	)
	mux.Handle(
		"POST /admin/users/{id}/sessions/revoke",
		browserAuthn(adminAuthz(handler.RevokeAdminUserSessions(userUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/users/{id}/oidc/remove",
		browserAuthn(adminAuthz(handler.RemoveAdminOIDCIdentity(userUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/oidc/pending/{id}/approve",
		browserAuthn(adminAuthz(handler.ApprovePendingOIDCIdentity(userUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/oidc/pending/{id}/link",
		browserAuthn(adminAuthz(handler.LinkPendingOIDCIdentity(userUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/oidc/pending/{id}/reject",
		browserAuthn(adminAuthz(handler.RejectPendingOIDCIdentity(userUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/oidc/pending/{id}/reopen",
		browserAuthn(adminAuthz(handler.ReopenPendingOIDCIdentity(userUseCases, logger))),
	)
	mux.Handle("POST /admin/groups", browserAuthn(adminAuthz(handler.CreateAdminGroup(groupUseCases, logger))))
	mux.Handle(
		"POST /admin/navigation",
		browserAuthn(adminAuthz(handler.SaveAdminNavigationIcon(navigationUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/bin/restore/{slug...}",
		browserAuthn(adminAuthz(handler.RestoreAdminPage(recycleBinUseCases, logger))),
	)
	mux.Handle(
		"POST /admin/bin/delete/{slug...}",
		browserAuthn(adminAuthz(handler.PermanentlyDeleteAdminPage(recycleBinUseCases, logger))),
	)
	mux.Handle("GET /api/admin/users", apiAuthn(adminAuthz(handler.SearchAdminUsers(userUseCases, logger))))
	mux.Handle("GET /api/icons", apiAuthn(editorAuthz(handler.SearchIcons(views.IconCatalog()))))
	mux.Handle(
		"GET /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(handler.AdminGroupMembers(groupUseCases, logger))),
	)
	mux.Handle(
		"POST /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(handler.AddAdminGroupMember(groupUseCases, userUseCases, logger))),
	)
	mux.Handle(
		"DELETE /api/admin/groups/{id}/members/{userID}",
		apiAuthn(adminAuthz(handler.RemoveAdminGroupMember(groupUseCases, logger))),
	)
	mux.Handle("POST /admin/groups/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminGroup(groupUseCases, logger))))
	mux.Handle(
		"POST /admin/tags/{id}/delete",
		browserAuthn(adminAuthz(handler.DeleteAdminTag(administrationUseCases, logger))),
	)
	mux.Handle("POST /admin/tokens", browserAuthn(adminAuthz(handler.CreateAdminToken(tokenUseCases, logger))))
	mux.Handle("DELETE /admin/tokens/{id}", browserAuthn(adminAuthz(handler.DeleteAdminToken(tokenUseCases, logger))))
	mux.Handle(
		"POST /admin/export",
		browserAuthn(adminAuthz(handler.ExportPages(catalogUseCases, navigationUseCases, mediaUseCases, logger))),
	)
	mux.Handle(
		"GET /export/markdown/{slug...}",
		browserAuthn(pageViewAuthz(handler.ExportPageMarkdown(catalogUseCases, mediaUseCases, logger))),
	)
	exportPDF := browserAuthn(pageViewAuthz(handler.ExportPagePDF(
		catalogUseCases, settingsUseCases, navigationUseCases,
		mediaUseCases, accessUseCases, renderer, views, logger,
	)))
	mux.Handle("GET /export/pdf/{slug...}", exportPDF)
	mux.Handle("POST /export/pdf/{slug...}", exportPDF)
	mux.Handle("POST /export/preview/{slug...}", browserAuthn(pageViewAuthz(handler.PreviewPageExport(
		catalogUseCases, settingsUseCases, navigationUseCases,
		mediaUseCases, accessUseCases, renderer, logger,
	))))
	mux.Handle("POST /pages/delete/{slug...}", browserAuthn(adminAuthz(handler.DeletePageForm(pageUseCases, views))))
	mux.Handle("POST /pages/move/{slug...}", browserAuthn(editorAuthz(pageEditAuthz(handler.MovePageForm(pageUseCases, accessUseCases, logger)))))
	mux.Handle("POST /pages/review/{slug...}", browserAuthn(editorAuthz(pageEditAuthz(handler.ReviewPageForm(pageUseCases, logger)))))
	mux.Handle("POST /pages/approval/request/{slug...}", browserAuthn(editorAuthz(pageEditAuthz(handler.RequestPageReview(pageUseCases, logger)))))
	mux.Handle("POST /pages/approval/update/{id}", browserAuthn(editorAuthz(handler.UpdatePageReview(pageUseCases, logger))))
	mux.Handle("POST /pages/approval/cancel/{id}", browserAuthn(editorAuthz(handler.CancelPageReview(pageUseCases, logger))))
	mux.Handle("POST /pages/approval/decide/{id}", browserAuthn(editorAuthz(handler.DecidePageReview(pageUseCases, logger))))
	mux.Handle("POST /page-comments/{slug...}", browserAuthn(pageViewAuthz(handler.AddPageComment(pageUseCases, views))))
	mux.Handle(
		"POST /page-comments/resolve/{id}",
		browserAuthn(editorAuthz(handler.ResolvePageComment(pageUseCases, views))),
	)
	mux.Handle(
		"GET /pages/new",
		browserAuthn(editorAuthz(pageEditAuthz(handler.EditPage(
			viewDataUseCases,
			catalogUseCases,
			groupUseCases,
			templateUseCases,
			accessUseCases,
			views,
		)))),
	)
	mux.Handle(
		"GET /edit/{slug...}",
		browserAuthn(editorAuthz(pageEditAuthz(handler.EditPage(
			viewDataUseCases,
			catalogUseCases,
			groupUseCases,
			templateUseCases,
			accessUseCases,
			views,
		)))),
	)
	mux.Handle(
		"POST /pages",
		browserAuthn(editorAuthz(handler.SavePageForm(pageUseCases, draftUseCases, templateUseCases, accessUseCases, views))),
	)
	mux.Handle("POST /pages/{slug...}", browserAuthn(pageViewAuthz(handler.FavoritePage(catalogUseCases, views))))
	mux.Handle("POST /page-watch/{slug...}", browserAuthn(pageViewAuthz(handler.WatchPage(catalogUseCases, views))))
	mux.Handle("GET /revisions/{slug...}", browserAuthn(pageViewAuthz(handler.RevisionHistory(catalogUseCases, views))))
	mux.Handle(
		"POST /revisions/{number}/restore/{slug...}",
		browserAuthn(editorAuthz(pageEditAuthz(handler.RestoreRevision(pageUseCases, views)))),
	)
	mux.Handle(
		"GET /pages/{slug...}",
		browserAuthn(pageViewAuthz(handler.ViewPage(
			viewDataUseCases,
			catalogUseCases,
			accessUseCases,
			pageUseCases,
			renderer,
			views,
		))),
	)

	// API routes accept either bearer-token or browser authentication.
	mux.Handle("GET /api/pages", apiAuthn(handler.ListPages(catalogUseCases, accessUseCases, logger)))
	mux.Handle("POST /api/pages", apiAuthn(editorAuthz(handler.SavePage(pageUseCases, accessUseCases, logger))))
	mux.Handle(
		"POST /api/preview",
		apiAuthn(editorAuthz(handler.PreviewMarkdown(
			navigationUseCases,
			catalogUseCases,
			accessUseCases,
			renderer,
			logger,
		))),
	)
	mux.Handle("GET /api/drafts/{key}", apiAuthn(editorAuthz(handler.GetPageDraft(draftUseCases, logger))))
	mux.Handle("PUT /api/drafts/{key}", apiAuthn(editorAuthz(handler.SavePageDraft(draftUseCases, logger))))
	mux.Handle("DELETE /api/drafts/{key}", apiAuthn(editorAuthz(handler.DeletePageDraft(draftUseCases, logger))))
	mux.Handle("GET /api/pages/{slug...}", apiAuthn(pageViewAuthz(handler.GetPage(catalogUseCases, logger))))
	mux.Handle("PUT /api/pages/{slug...}", apiAuthn(editorAuthz(handler.SavePage(pageUseCases, accessUseCases, logger))))
	mux.Handle("DELETE /api/pages/{slug...}", apiAuthn(adminAuthz(pageEditAuthz(handler.DeletePage(pageUseCases, logger)))))
	mux.Handle(
		"DELETE /api/admin/bin/{slug...}",
		apiAuthn(adminAuthz(handler.PermanentlyDeletePage(recycleBinUseCases, logger))),
	)
	mux.Handle("GET /api/search", apiAuthn(handler.SearchAPI(catalogUseCases, accessUseCases, logger)))
	mux.Handle("GET /api/graph", apiAuthn(handler.KnowledgeGraphAPI(knowledgeUseCases, accessUseCases, logger)))
	mux.Handle(
		"GET /api/editor/catalog",
		apiAuthn(editorAuthz(handler.EditorCatalog(navigationUseCases, catalogUseCases, renderer.PluginManager(), logger))),
	)
	mux.Handle("GET /api/mentions/users", apiAuthn(handler.MentionUsers(userUseCases, logger)))
	mux.Handle("GET /api/notifications", apiAuthn(handler.NotificationsAPI(notificationUseCases, logger)))
	mux.Handle("POST /notifications/{id}/open", browserAuthn(handler.OpenNotification(notificationUseCases, logger)))
	mux.Handle(
		"POST /api/notifications/{id}/read",
		apiAuthn(handler.MarkNotificationRead(notificationUseCases, logger)),
	)
	mux.Handle("GET /api/tags", apiAuthn(handler.Tags(catalogUseCases, logger)))
	mux.Handle("GET /api/groups", apiAuthn(handler.GroupsAPI(groupUseCases, logger)))
	mux.Handle("GET /api/images", apiAuthn(editorAuthz(handler.ListImages(mediaUseCases, logger))))
	mux.Handle("GET /api/attachments", apiAuthn(editorAuthz(handler.ListAttachments(mediaUseCases, logger))))
	mux.Handle("POST /api/attachments", apiAuthn(editorAuthz(handler.UploadAttachment(mediaUseCases, logger))))
	mux.Handle(
		"DELETE /api/attachments/{id}",
		apiAuthn(editorAuthz(handler.DeleteAttachment(mediaUseCases, logger))),
	)
	mux.Handle("POST /api/images", apiAuthn(editorAuthz(handler.UploadImage(mediaUseCases, logger))))
	mux.Handle("DELETE /api/images/{id}", apiAuthn(editorAuthz(handler.DeleteImage(mediaUseCases, logger))))
	mux.Handle("GET /api/recent", apiAuthn(handler.Recent(catalogUseCases, accessUseCases, logger)))
	mux.Handle(
		"POST /api/admin/export",
		apiAuthn(adminAuthz(handler.ExportPages(catalogUseCases, navigationUseCases, mediaUseCases, logger))),
	)

	// Keep unknown API paths machine-readable while browser navigation gets the themed 404 page.
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})
	mux.Handle("GET /", browserAuthn(handler.NotFound(viewDataUseCases, views)))
}
