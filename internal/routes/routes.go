package routes

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
)

// routeRegistrar groups route dependencies and derived page-access middleware.
type routeRegistrar struct {
	// mux receives all application route registrations.
	mux *http.ServeMux
	// config contains fully constructed handler dependencies.
	config Config
	// policies contains authentication and role middleware.
	policies routePolicies
	// pageViewAuthz enforces inherited page-view access.
	pageViewAuthz middleware.Middleware
	// pageEditAuthz enforces inherited page-edit access.
	pageEditAuthz middleware.Middleware
}

// addRoutes registers the complete HTTP surface by functional area.
func addRoutes(mux *http.ServeMux, config Config, policies routePolicies) {
	routes := routeRegistrar{
		mux:           mux,
		config:        config,
		policies:      policies,
		pageViewAuthz: middleware.RequirePageView(config.Access),
		pageEditAuthz: middleware.RequirePageEdit(config.Access),
	}

	routes.addPublicRoutes()
	routes.addBrowserRoutes()
	routes.addAdminRoutes()
	routes.addPageRoutes()
	routes.addAPIRoutes()
	routes.addFallbackRoutes()
}

// addPublicRoutes registers infrastructure, assets, login, and setup endpoints.
func (r routeRegistrar) addPublicRoutes() {
	config := r.config
	browserAuth := config.BrowserAuth
	renderer := config.Renderer

	r.mux.HandleFunc("GET /plugins/styles.css", handler.PluginPresentationStyles(renderer.PluginManager()))
	r.mux.HandleFunc("GET /plugins/runtime.js", handler.PluginBrowserRuntime(config.Assets))
	r.mux.HandleFunc("GET /plugins/{pluginID}/{digest}/assets/{asset...}", handler.PluginAssets(renderer.PluginManager()))
	r.mux.HandleFunc("GET /plugins/{pluginID}/{digest}/frames/{frame}", handler.PluginFrame(renderer.PluginManager()))
	r.mux.HandleFunc("GET /healthz", handler.Health(config.System))
	r.mux.Handle("GET /robots.txt", handler.Robots(config.Settings, config.Views, config.Logger))
	r.mux.Handle("GET /sitemap.xml", handler.Sitemap(config.Settings, config.Catalog, config.Access, config.Views, config.Logger))
	r.mux.Handle("GET /assets/", handler.Assets(config.Assets))
	r.mux.Handle("GET /sw.js", handler.ServiceWorker(config.Assets))
	r.mux.Handle("GET /auth/login", browserAuth.Login)

	localLogin := handler.LocalLogin(config.Settings, config.System, browserAuth, config.Views)
	r.mux.Handle("GET /auth/local", localLogin)
	r.mux.Handle("POST /auth/local", localLogin)

	setup := handler.Setup(config.Settings, config.System, browserAuth, config.Views)
	r.mux.Handle("GET /setup", setup)
	r.mux.Handle("POST /setup", setup)

	if browserAuth.Callback != nil {
		r.mux.Handle("GET /auth/callback", browserAuth.Callback)
	}
}

// addBrowserRoutes registers authenticated user, media, and preference endpoints.
func (r routeRegistrar) addBrowserRoutes() {
	config := r.config
	browserAuthn := r.policies.browserAuthn
	mediaAuthn := r.policies.mediaAuthn

	r.mux.Handle("POST /auth/logout", browserAuthn(auth.Logout(config.BrowserAuth.Local)))
	r.mux.Handle("GET /{$}", browserAuthn(handler.Home(config.ViewData, config.Catalog, config.Drafts, config.Access, config.Renderer, config.Views)))
	r.mux.Handle("GET /search", browserAuthn(handler.Search(config.ViewData, config.Catalog, config.Access, config.Views)))
	r.mux.Handle("GET /graph", browserAuthn(handler.KnowledgeGraphPage(config.ViewData, config.Views)))
	r.mux.Handle("GET /p/{id}", browserAuthn(handler.PagePermalink(config.Catalog, config.Logger)))
	r.mux.Handle(
		"GET /settings",
		browserAuthn(handler.Settings(config.ViewData, config.Users, config.Tokens, config.Media, config.BrowserAuth.Local, config.Views)),
	)

	r.mux.Handle("GET /media/{id}/{name...}", mediaAuthn(handler.ServeImage(config.Media, config.Logger)))
	r.mux.Handle("GET /attachments/{id}/{name...}", mediaAuthn(handler.ServeAttachment(config.Media, config.Logger)))

	r.mux.Handle("POST /settings/preferences", browserAuthn(handler.SavePreferences(config.Preferences, config.Renderer.PluginManager(), config.Views)))
	r.mux.Handle("POST /settings/local-password", browserAuthn(handler.ChangeLocalPassword(config.BrowserAuth.Local, config.Logger)))
	r.mux.Handle(
		"POST /settings/preferences/page-contents",
		browserAuthn(handler.SavePageContentsPreference(config.Preferences, config.Views)),
	)
	r.mux.Handle(
		"POST /settings/preferences/navigation-state",
		browserAuthn(handler.SaveNavigationState(config.Preferences, config.Logger)),
	)
	r.mux.Handle(
		"POST /settings/preferences/sidebar-width",
		browserAuthn(handler.SaveSidebarWidth(config.Preferences, config.Logger)),
	)
	r.mux.Handle("POST /settings/saved-searches", browserAuthn(handler.CreateSavedSearch(config.Knowledge, config.Logger)))
	r.mux.Handle(
		"POST /settings/saved-searches/{id}/delete",
		browserAuthn(handler.DeleteSavedSearch(config.Knowledge, config.Logger)),
	)
	r.mux.Handle("POST /settings/tokens", browserAuthn(handler.CreatePersonalToken(config.Tokens, config.Logger)))
	r.mux.Handle("DELETE /settings/tokens/{id}", browserAuthn(handler.DeletePersonalToken(config.Tokens, config.Logger)))
	r.mux.Handle("POST /notifications/{id}/open", browserAuthn(handler.OpenNotification(config.Notifications, config.Logger)))
}

// addAdminRoutes registers administrator pages, mutations, and administrator APIs.
func (r routeRegistrar) addAdminRoutes() {
	config := r.config
	browserAuthn := r.policies.browserAuthn
	apiAuthn := r.policies.apiAuthn
	adminAuthz := r.policies.adminAuthz

	pluginsAdmin := handler.NewAdminPlugins(config.Renderer.PluginManager(), config.ViewData, config.Views)
	r.mux.Handle("GET /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.List))))
	r.mux.Handle("POST /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Install))))
	r.mux.Handle("POST /admin/plugins/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Action))))

	r.mux.Handle("GET /admin", browserAuthn(adminAuthz(handler.Administration(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle(
		"GET /admin/configuration",
		browserAuthn(adminAuthz(handler.AdminConfiguration(config.ViewData, config.Groups, config.Users, config.Settings, config.Views))),
	)
	r.mux.Handle(
		"GET /admin/health",
		browserAuthn(adminAuthz(handler.AdminDocumentationHealth(config.ViewData, config.Administration, config.Views))),
	)
	r.mux.Handle(
		"GET /admin/templates",
		browserAuthn(adminAuthz(handler.AdminPageTemplates(config.ViewData, config.Templates, config.Groups, config.Views))),
	)
	r.mux.Handle("GET /admin/permissions", browserAuthn(adminAuthz(handler.AdminPageAccess(config.ViewData, config.Access, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/webhooks", browserAuthn(adminAuthz(handler.AdminWebhooks(config.ViewData, config.Webhooks, config.Views))))
	r.mux.Handle("POST /admin/webhooks", browserAuthn(adminAuthz(handler.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}", browserAuthn(adminAuthz(handler.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/test", browserAuthn(adminAuthz(handler.TestAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", browserAuthn(adminAuthz(handler.RevealAdminWebhookHeader(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/permissions", browserAuthn(adminAuthz(handler.SaveAdminPageAccess(config.Access, config.Logger))))
	r.mux.Handle("POST /admin/permissions/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminPageAccess(config.Access, config.Logger))))
	r.mux.Handle("GET /admin/audit", browserAuthn(adminAuthz(handler.AdminAudit(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle("GET /admin/pages", browserAuthn(adminAuthz(handler.AdminPages(config.ViewData, config.Catalog, config.Groups, config.Views))))
	r.mux.Handle(
		"POST /admin/pages/bulk",
		browserAuthn(adminAuthz(handler.BulkAdminPages(config.Pages, config.Catalog, config.Media, config.Logger))),
	)
	r.mux.Handle("GET /admin/import", browserAuthn(adminAuthz(handler.AdminImport(config.ViewData, config.Views))))
	r.mux.Handle(
		"POST /admin/import",
		browserAuthn(adminAuthz(handler.ImportPagesWithPortableArchive(config.Pages, config.Media, config.Groups, config.Logger))),
	)
	r.mux.Handle("POST /admin/templates", browserAuthn(adminAuthz(handler.CreateAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("POST /admin/templates/{id}", browserAuthn(adminAuthz(handler.UpdateAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("POST /admin/templates/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("GET /admin/users", browserAuthn(adminAuthz(handler.AdminUsers(config.ViewData, config.Users, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/groups", browserAuthn(adminAuthz(handler.AdminGroups(config.ViewData, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/navigation", browserAuthn(adminAuthz(handler.AdminNavigation(config.ViewData, config.Navigation, config.Views))))
	r.mux.Handle("GET /admin/bin", browserAuthn(adminAuthz(handler.AdminBin(config.ViewData, config.RecycleBin, config.Views))))
	r.mux.Handle("GET /admin/tags", browserAuthn(adminAuthz(handler.AdminTags(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle("GET /admin/tokens", browserAuthn(adminAuthz(handler.AdminTokens(config.ViewData, config.Users, config.Tokens, config.Views))))
	r.mux.Handle("GET /admin/exports", browserAuthn(adminAuthz(handler.AdminExports(config.ViewData, config.Navigation, config.Views))))
	r.mux.Handle("GET /admin/images", browserAuthn(adminAuthz(handler.AdminImages(config.ViewData, config.Media, config.Views))))
	r.mux.Handle("POST /admin/settings", browserAuthn(adminAuthz(handler.SaveAdminSettings(config.Settings, config.Logger))))
	r.mux.Handle("POST /admin/pdf", browserAuthn(adminAuthz(handler.SaveAdminPDFSettings(config.Settings, config.Logger))))
	r.mux.Handle("POST /admin/pdf/test", browserAuthn(adminAuthz(handler.TestAdminPDFService(config.Settings, config.Logger))))
	r.mux.Handle("POST /admin/pdf/headers/{id}/reveal", browserAuthn(adminAuthz(handler.RevealAdminPDFHeader(config.Settings, config.Logger))))
	r.mux.Handle(
		"POST /admin/authentication",
		browserAuthn(adminAuthz(handler.SaveAdminAuthentication(config.Settings, config.BrowserAuth, config.Views))),
	)

	r.mux.Handle("POST /admin/users/{id}", browserAuthn(adminAuthz(handler.UpdateAdminUser(config.Users, config.Views, config.Logger))))
	r.mux.Handle("POST /admin/users/{id}/sessions/revoke", browserAuthn(adminAuthz(handler.RevokeAdminUserSessions(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/users/{id}/oidc/remove", browserAuthn(adminAuthz(handler.RemoveAdminOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/approve", browserAuthn(adminAuthz(handler.ApprovePendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/link", browserAuthn(adminAuthz(handler.LinkPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/reject", browserAuthn(adminAuthz(handler.RejectPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/reopen", browserAuthn(adminAuthz(handler.ReopenPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/groups", browserAuthn(adminAuthz(handler.CreateAdminGroup(config.Groups, config.Logger))))
	r.mux.Handle("POST /admin/groups/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminGroup(config.Groups, config.Logger))))
	r.mux.Handle("POST /admin/navigation", browserAuthn(adminAuthz(handler.SaveAdminNavigationIcon(config.Navigation, config.Logger))))
	r.mux.Handle("POST /admin/bin/restore/{slug...}", browserAuthn(adminAuthz(handler.RestoreAdminPage(config.RecycleBin, config.Logger))))
	r.mux.Handle("POST /admin/bin/delete/{slug...}", browserAuthn(adminAuthz(handler.PermanentlyDeleteAdminPage(config.RecycleBin, config.Logger))))
	r.mux.Handle("POST /admin/tags/{id}/delete", browserAuthn(adminAuthz(handler.DeleteAdminTag(config.Administration, config.Logger))))
	r.mux.Handle("POST /admin/tokens", browserAuthn(adminAuthz(handler.CreateAdminToken(config.Tokens, config.Logger))))
	r.mux.Handle("DELETE /admin/tokens/{id}", browserAuthn(adminAuthz(handler.DeleteAdminToken(config.Tokens, config.Logger))))
	r.mux.Handle(
		"POST /admin/export",
		browserAuthn(adminAuthz(handler.ExportPortablePages(config.Catalog, config.Navigation, config.Media, config.Logger))),
	)

	r.mux.Handle("GET /api/admin/users", apiAuthn(adminAuthz(handler.SearchAdminUsers(config.Users, config.Logger))))
	r.mux.Handle(
		"GET /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(handler.AdminGroupMembers(config.Groups, config.Logger))),
	)
	r.mux.Handle(
		"POST /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(handler.AddAdminGroupMember(config.Groups, config.Users, config.Logger))),
	)
	r.mux.Handle(
		"DELETE /api/admin/groups/{id}/members/{userID}",
		apiAuthn(adminAuthz(handler.RemoveAdminGroupMember(config.Groups, config.Logger))),
	)
	r.mux.Handle(
		"POST /api/admin/export",
		apiAuthn(adminAuthz(handler.ExportPortablePages(config.Catalog, config.Navigation, config.Media, config.Logger))),
	)
	r.mux.Handle(
		"DELETE /api/admin/bin/{slug...}",
		apiAuthn(adminAuthz(handler.PermanentlyDeletePage(config.RecycleBin, config.Logger))),
	)
}

// addPageRoutes registers browser page workflows, collaboration, and export endpoints.
func (r routeRegistrar) addPageRoutes() {
	config := r.config
	browserAuthn := r.policies.browserAuthn
	adminAuthz := r.policies.adminAuthz
	editorAuthz := r.policies.editorAuthz
	r.mux.Handle(
		"POST /plugins/actions/{pluginID}/{moduleID}/{actionID}",
		browserAuthn(handler.PluginWidgetCommand(config.Catalog, config.Navigation, config.Access, config.Renderer)),
	)

	r.mux.Handle(
		"GET /export/markdown/{slug...}",
		browserAuthn(r.pageViewAuthz(handler.ExportPageMarkdown(config.Catalog, config.Media, config.Logger))),
	)
	r.mux.Handle(
		"POST /export/plugin/{pluginID}/{moduleID}/{slug...}",
		browserAuthn(r.pageViewAuthz(handler.ExportPagePlugin(config.Catalog, config.Navigation, config.Access, config.Renderer, config.Logger))),
	)
	exportPDF := browserAuthn(r.pageViewAuthz(handler.ExportPagePDF(
		config.Catalog,
		config.Settings,
		config.Navigation,
		config.Media,
		config.Access,
		config.Renderer,
		config.Views,
		config.Logger,
	)))
	r.mux.Handle("GET /export/pdf/{slug...}", exportPDF)
	r.mux.Handle("POST /export/pdf/{slug...}", exportPDF)
	r.mux.Handle(
		"POST /export/preview/{slug...}",
		browserAuthn(r.pageViewAuthz(handler.PreviewPageExport(
			config.Catalog,
			config.Settings,
			config.Navigation,
			config.Media,
			config.Access,
			config.Renderer,
			config.Logger,
		))),
	)

	r.mux.Handle("POST /pages/delete/{slug...}", browserAuthn(adminAuthz(handler.DeletePageForm(config.Pages, config.Views))))
	r.mux.Handle("POST /pages/move/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(handler.MovePageForm(config.Pages, config.Access, config.Logger)))))
	r.mux.Handle("POST /pages/review/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(handler.ReviewPageForm(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/request/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(handler.RequestPageReview(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/update/{id}", browserAuthn(editorAuthz(handler.UpdatePageReview(config.Pages, config.Logger))))
	r.mux.Handle("POST /pages/approval/cancel/{id}", browserAuthn(editorAuthz(handler.CancelPageReview(config.Pages, config.Logger))))
	r.mux.Handle("POST /pages/approval/decide/{id}", browserAuthn(editorAuthz(handler.DecidePageReview(config.Pages, config.Logger))))
	r.mux.Handle("POST /page-comments/{slug...}", browserAuthn(r.pageViewAuthz(handler.AddPageComment(config.Pages, config.Views))))
	r.mux.Handle("POST /page-comments/resolve/{id}", browserAuthn(editorAuthz(handler.ResolvePageComment(config.Pages, config.Views))))

	editPage := handler.EditPage(config.ViewData, config.Catalog, config.Groups, config.Templates, config.Access, config.Views)
	r.mux.Handle("GET /pages/new", browserAuthn(editorAuthz(r.pageEditAuthz(editPage))))
	r.mux.Handle("GET /edit/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(editPage))))
	r.mux.Handle(
		"POST /pages",
		browserAuthn(editorAuthz(handler.SavePageForm(config.Pages, config.Drafts, config.Templates, config.Access, config.Views))),
	)
	r.mux.Handle("POST /pages/{slug...}", browserAuthn(r.pageViewAuthz(handler.FavoritePage(config.Catalog, config.Views))))
	r.mux.Handle("POST /page-watch/{slug...}", browserAuthn(r.pageViewAuthz(handler.WatchPage(config.Catalog, config.Views))))
	r.mux.Handle("GET /revisions/{slug...}", browserAuthn(r.pageViewAuthz(handler.RevisionHistory(config.Catalog, config.Views))))
	r.mux.Handle(
		"POST /revisions/{number}/restore/{slug...}",
		browserAuthn(editorAuthz(r.pageEditAuthz(handler.RestoreRevision(config.Pages, config.Views)))),
	)
	r.mux.Handle(
		"GET /pages/{slug...}",
		browserAuthn(r.pageViewAuthz(handler.ViewPage(
			config.ViewData,
			config.Catalog,
			config.Access,
			config.Pages,
			config.Renderer,
			config.Views,
		))),
	)
}

// addAPIRoutes registers authenticated machine-readable application endpoints.
func (r routeRegistrar) addAPIRoutes() {
	config := r.config
	apiAuthn := r.policies.apiAuthn
	adminAuthz := r.policies.adminAuthz
	editorAuthz := r.policies.editorAuthz

	r.mux.Handle("GET /api/icons", apiAuthn(editorAuthz(handler.SearchIcons(config.Views.IconCatalog()))))
	r.mux.Handle("GET /api/pages", apiAuthn(handler.ListPages(config.Catalog, config.Access, config.Logger)))
	r.mux.Handle("POST /api/pages", apiAuthn(editorAuthz(handler.SavePage(config.Pages, config.Access, config.Logger))))
	r.mux.Handle(
		"POST /api/preview",
		apiAuthn(editorAuthz(handler.PreviewMarkdown(
			config.Navigation,
			config.Catalog,
			config.Access,
			config.Renderer,
			config.Logger,
		))),
	)
	r.mux.Handle("GET /api/drafts/{key}", apiAuthn(editorAuthz(handler.GetPageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("PUT /api/drafts/{key}", apiAuthn(editorAuthz(handler.SavePageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("DELETE /api/drafts/{key}", apiAuthn(editorAuthz(handler.DeletePageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("GET /api/pages/{slug...}", apiAuthn(r.pageViewAuthz(handler.GetPage(config.Catalog, config.Logger))))
	r.mux.Handle("PUT /api/pages/{slug...}", apiAuthn(editorAuthz(handler.SavePage(config.Pages, config.Access, config.Logger))))
	r.mux.Handle("DELETE /api/pages/{slug...}", apiAuthn(adminAuthz(r.pageEditAuthz(handler.DeletePage(config.Pages, config.Logger)))))
	r.mux.Handle("GET /api/search", apiAuthn(handler.SearchAPI(config.Catalog, config.Access, config.Logger)))
	r.mux.Handle("GET /api/graph", apiAuthn(handler.KnowledgeGraphAPI(config.Knowledge, config.Access, config.Logger)))
	r.mux.Handle(
		"GET /api/editor/catalog",
		apiAuthn(editorAuthz(handler.EditorCatalog(config.Navigation, config.Catalog, config.Renderer.PluginManager(), config.Logger))),
	)
	r.mux.Handle("GET /api/mentions/users", apiAuthn(handler.MentionUsers(config.Users, config.Logger)))
	r.mux.Handle("GET /api/notifications", apiAuthn(handler.NotificationsAPI(config.Notifications, config.Logger)))
	r.mux.Handle("POST /api/notifications/{id}/read", apiAuthn(handler.MarkNotificationRead(config.Notifications, config.Logger)))
	r.mux.Handle("GET /api/tags", apiAuthn(handler.Tags(config.Catalog, config.Logger)))
	r.mux.Handle("GET /api/groups", apiAuthn(handler.GroupsAPI(config.Groups, config.Logger)))
	r.mux.Handle("GET /api/images", apiAuthn(editorAuthz(handler.ListImages(config.Media, config.Logger))))
	r.mux.Handle("GET /api/attachments", apiAuthn(editorAuthz(handler.ListAttachments(config.Media, config.Logger))))
	r.mux.Handle("POST /api/attachments", apiAuthn(editorAuthz(handler.UploadAttachment(config.Media, config.Logger))))
	r.mux.Handle("DELETE /api/attachments/{id}", apiAuthn(editorAuthz(handler.DeleteAttachment(config.Media, config.Logger))))
	r.mux.Handle("POST /api/images", apiAuthn(editorAuthz(handler.UploadImage(config.Media, config.Logger))))
	r.mux.Handle("DELETE /api/images/{id}", apiAuthn(editorAuthz(handler.DeleteImage(config.Media, config.Logger))))
	r.mux.Handle("GET /api/recent", apiAuthn(handler.Recent(config.Catalog, config.Access, config.Logger)))
}

// addFallbackRoutes registers machine-readable API and themed browser not-found handlers.
func (r routeRegistrar) addFallbackRoutes() {
	r.mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})
	r.mux.Handle("GET /", r.policies.browserAuthn(handler.NotFound(r.config.ViewData, r.config.Views)))
}
