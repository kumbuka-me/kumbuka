package app

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/http/routes"
)

const (
	pageCommentSuggestionApplyPattern    = "POST /page-comments/suggestions/apply/{id}/{slug...}"
	pageReviewSuggestionApplyPattern     = "POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}"
	pageReviewSuggestionsApplyAllPattern = "POST /reviews/{id}/suggestions/apply-all/{slug...}"
)

// routeRegistrar binds fully constructed endpoints to HTTP patterns.
type routeRegistrar struct {
	// router registers endpoints and applies HTTP authentication/role policies.
	router *routes.Router
	// config contains application dependencies used only while constructing endpoints.
	config httpConfig
}

// addRoutes registers the complete HTTP surface by functional area.
func addRoutes(router *routes.Router, config httpConfig) {
	registrar := routeRegistrar{
		router: router,
		config: config,
	}

	registrar.addPublicRoutes()
	registrar.addBrowserRoutes()
	registrar.addAdminRoutes()
	registrar.addPageRoutes()
	registrar.addAPIRoutes()
	registrar.addFallbackRoutes()
}

// addPublicRoutes registers infrastructure, assets, login, and setup endpoints.
func (r routeRegistrar) addPublicRoutes() {
	config := r.config
	browserAuth := config.BrowserAuth
	renderer := config.Renderer

	r.router.HandleFunc("GET /plugins/styles.css", endpoint.PluginPresentationStyles(renderer.PluginManager()))
	r.router.HandleFunc("GET /plugins/runtime.js", endpoint.PluginBrowserRuntime(config.Assets))
	r.router.HandleFunc("GET /plugins/{pluginID}/{digest}/assets/{asset...}", endpoint.PluginAssets(renderer.PluginManager()))
	r.router.HandleFunc("GET /plugins/{pluginID}/{digest}/frames/{frame}", endpoint.PluginFrame(renderer.PluginManager()))
	r.router.HandleFunc("GET /healthz", endpoint.Health(config.System))
	r.router.Handle("GET /robots.txt", endpoint.Robots(config.Settings, config.Views, config.Logger))
	r.router.Handle("GET /sitemap.xml", endpoint.Sitemap(config.Settings, config.PageDirectory, config.Views, config.Logger))
	r.router.Handle("GET /assets/", endpoint.Assets(config.Assets))
	r.router.Handle("GET /sw.js", endpoint.ServiceWorker(config.Assets))
	r.router.Handle("GET /brand/logo", endpoint.BrandLogo(config.Settings, config.Assets, config.Logger))
	r.router.Handle("GET /auth/login", browserAuth.Login)

	localLogin := endpoint.LocalLogin(config.Settings, config.System, browserAuth, config.Views)
	r.router.Handle("GET /auth/local", localLogin)
	r.router.Handle("POST /auth/local", localLogin)

	setup := endpoint.Setup(config.Settings, config.System, browserAuth, config.Views)
	r.router.Handle("GET /setup", setup)
	r.router.Handle("POST /setup", setup)

	if browserAuth.Callback != nil {
		r.router.Handle("GET /auth/callback", browserAuth.Callback)
	}
}

// addBrowserRoutes registers authenticated user, media, and preference endpoints.
func (r routeRegistrar) addBrowserRoutes() {
	config := r.config
	browserAuthn := r.router.Browser
	mediaAuthn := r.router.Media

	r.router.Handle("POST /auth/logout", browserAuthn(auth.Logout(config.BrowserAuth.Local)))
	r.router.Handle("GET /{$}", browserAuthn(endpoint.Home(config.BrowserContext, config.Home, config.Renderer, config.Views)))
	r.router.Handle("GET /search", browserAuthn(endpoint.Search(config.BrowserContext, config.PageSearch, config.Views)))
	r.router.Handle("GET /graph", browserAuthn(endpoint.KnowledgeGraphPage(config.BrowserContext, config.Views)))
	r.router.Handle("GET /p/{id}", browserAuthn(endpoint.PagePermalink(config.PageLookup, config.Logger)))
	r.router.Handle(
		"GET /settings",
		browserAuthn(endpoint.Settings(config.BrowserContext, config.Users, config.Tokens, config.Media, config.BrowserAuth.Local, config.Views)),
	)

	r.router.Handle("GET /media/{id}/{name...}", mediaAuthn(endpoint.ServeImage(config.Media, config.Logger)))
	r.router.Handle("GET /attachments/{id}/{name...}", mediaAuthn(endpoint.ServeAttachment(config.Media, config.Logger)))

	r.router.Handle("POST /settings/preferences", browserAuthn(endpoint.SavePreferences(config.Preferences, config.Renderer.PluginManager(), config.Views)))
	r.router.Handle("POST /settings/local-password", browserAuthn(endpoint.ChangeLocalPassword(config.BrowserAuth.Local, config.Logger)))
	r.router.Handle(
		"POST /settings/preferences/page-contents",
		browserAuthn(endpoint.SavePageContentsPreference(config.Preferences, config.Views)),
	)
	r.router.Handle(
		"POST /settings/preferences/navigation-state",
		browserAuthn(endpoint.SaveNavigationState(config.Preferences, config.Logger)),
	)
	r.router.Handle(
		"POST /settings/preferences/sidebar-width",
		browserAuthn(endpoint.SaveSidebarWidth(config.Preferences, config.Logger)),
	)
	r.router.Handle("POST /settings/saved-searches", browserAuthn(endpoint.CreateSavedSearch(config.Knowledge, config.Logger)))
	r.router.Handle(
		"POST /settings/saved-searches/{id}/delete",
		browserAuthn(endpoint.DeleteSavedSearch(config.Knowledge, config.Logger)),
	)
	r.router.Handle("POST /settings/tokens", browserAuthn(endpoint.CreatePersonalToken(config.Tokens, config.Logger)))
	r.router.Handle("DELETE /settings/tokens/{id}", browserAuthn(endpoint.DeletePersonalToken(config.Tokens, config.Logger)))
	r.router.Handle("POST /notifications/{id}/open", browserAuthn(endpoint.OpenNotification(config.Notifications, config.Logger)))
}

// addAdminRoutes registers administrator pages, mutations, and administrator APIs.
func (r routeRegistrar) addAdminRoutes() {
	config := r.config
	browserAuthn := r.router.Browser
	apiAuthn := r.router.API
	adminAuthz := r.router.Admin

	pluginManager := config.Renderer.PluginManager()
	pluginsAdmin := endpoint.NewAdminPlugins(pluginManager, config.PluginUpdates, config.BrowserContext, config.Views)
	r.router.Handle("GET /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.List))))
	r.router.Handle("POST /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Install))))
	r.router.Handle("POST /admin/plugins/check-updates", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.CheckUpdates))))
	r.router.Handle("POST /admin/plugins/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Action))))
	pluginSettings := endpoint.NewAdminPluginSettings(pluginManager, config.BrowserContext, config.Views)
	r.router.Handle("GET /admin/plugin-settings/{pluginID}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Show))))
	r.router.Handle("POST /admin/plugin-settings/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Action))))

	r.router.Handle("GET /admin", browserAuthn(adminAuthz(endpoint.Administration(config.BrowserContext, config.Administration, config.Views))))
	r.router.Handle(
		"GET /admin/configuration",
		browserAuthn(adminAuthz(endpoint.AdminConfiguration(config.BrowserContext, config.Groups, config.Users, config.Settings, config.Views))),
	)
	r.router.Handle(
		"GET /admin/branding",
		browserAuthn(adminAuthz(endpoint.AdminBranding(config.BrowserContext, config.Views))),
	)
	r.router.Handle(
		"GET /admin/health",
		browserAuthn(adminAuthz(endpoint.AdminDocumentationHealth(config.BrowserContext, config.Administration, config.Views))),
	)
	r.router.Handle(
		"GET /admin/templates",
		browserAuthn(adminAuthz(endpoint.AdminPageTemplates(config.BrowserContext, config.Templates, config.Groups, config.Views))),
	)
	r.router.Handle("GET /admin/permissions", browserAuthn(adminAuthz(endpoint.AdminPageAccess(config.BrowserContext, config.Access, config.Groups, config.Views))))
	r.router.Handle("GET /admin/webhooks", browserAuthn(adminAuthz(endpoint.AdminWebhooks(config.BrowserContext, config.Webhooks, config.Views))))
	r.router.Handle("POST /admin/webhooks", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.router.Handle("POST /admin/webhooks/{id}", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.router.Handle("POST /admin/webhooks/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminWebhook(config.Webhooks, config.Logger))))
	r.router.Handle("POST /admin/webhooks/{id}/test", browserAuthn(adminAuthz(endpoint.TestAdminWebhook(config.Webhooks, config.Logger))))
	r.router.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminWebhookHeader(config.Webhooks, config.Logger))))
	r.router.Handle("POST /admin/permissions", browserAuthn(adminAuthz(endpoint.SaveAdminPageAccess(config.Access, config.Logger))))
	r.router.Handle("POST /admin/permissions/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageAccess(config.Access, config.Logger))))
	r.router.Handle("GET /admin/audit", browserAuthn(adminAuthz(endpoint.AdminAudit(config.BrowserContext, config.Administration, config.Views))))
	r.router.Handle("GET /admin/pages", browserAuthn(adminAuthz(endpoint.AdminPages(config.BrowserContext, config.PageDirectory, config.Groups, config.Views))))
	r.router.Handle(
		"POST /admin/pages/bulk",
		browserAuthn(adminAuthz(endpoint.BulkAdminPages(config.Pages, config.PageLookup, config.Media, config.Logger))),
	)
	r.router.Handle("GET /admin/import", browserAuthn(adminAuthz(endpoint.AdminImport(config.BrowserContext, config.Views))))
	r.router.Handle(
		"POST /admin/import",
		browserAuthn(adminAuthz(endpoint.ImportPagesWithPortableArchive(config.Pages, config.Media, config.Groups, config.Logger))),
	)
	r.router.Handle("POST /admin/templates", browserAuthn(adminAuthz(endpoint.CreateAdminPageTemplate(config.Templates, config.Logger))))
	r.router.Handle("POST /admin/templates/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminPageTemplate(config.Templates, config.Logger))))
	r.router.Handle("POST /admin/templates/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageTemplate(config.Templates, config.Logger))))
	r.router.Handle("GET /admin/users", browserAuthn(adminAuthz(endpoint.AdminUsers(config.BrowserContext, config.Users, config.Groups, config.Views))))
	r.router.Handle("GET /admin/groups", browserAuthn(adminAuthz(endpoint.AdminGroups(config.BrowserContext, config.Groups, config.Views))))
	r.router.Handle("GET /admin/navigation", browserAuthn(adminAuthz(endpoint.AdminNavigation(config.BrowserContext, config.Navigation, config.Views))))
	r.router.Handle("GET /admin/bin", browserAuthn(adminAuthz(endpoint.AdminBin(config.BrowserContext, config.RecycleBin, config.Views))))
	r.router.Handle("GET /admin/tags", browserAuthn(adminAuthz(endpoint.AdminTags(config.BrowserContext, config.Administration, config.Views))))
	r.router.Handle("GET /admin/tokens", browserAuthn(adminAuthz(endpoint.AdminTokens(config.BrowserContext, config.Users, config.Tokens, config.Views))))
	r.router.Handle("GET /admin/exports", browserAuthn(adminAuthz(endpoint.AdminExports(config.BrowserContext, config.Navigation, config.Views))))
	r.router.Handle("GET /admin/images", browserAuthn(adminAuthz(endpoint.AdminImages(config.BrowserContext, config.Media, config.Views))))
	r.router.Handle("POST /admin/settings", browserAuthn(adminAuthz(endpoint.SaveAdminSettings(config.Settings, config.Views, config.Logger))))
	r.router.Handle(
		"POST /admin/branding/logo",
		browserAuthn(adminAuthz(endpoint.SaveAdminBrandLogo(config.Settings, config.Logger))),
	)
	r.router.Handle(
		"POST /admin/branding/logo/reset",
		browserAuthn(adminAuthz(endpoint.ResetAdminBrandLogo(config.Settings, config.Logger))),
	)
	r.router.Handle("POST /admin/pdf", browserAuthn(adminAuthz(endpoint.SaveAdminPDFSettings(config.Settings, config.Logger))))
	r.router.Handle("POST /admin/pdf/test", browserAuthn(adminAuthz(endpoint.TestAdminPDFService(config.Settings, config.Logger))))
	r.router.Handle("POST /admin/pdf/headers/{id}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminPDFHeader(config.Settings, config.Logger))))
	r.router.Handle(
		"POST /admin/authentication",
		browserAuthn(adminAuthz(endpoint.SaveAdminAuthentication(config.Settings, config.BrowserAuth, config.Views))),
	)

	r.router.Handle("POST /admin/users/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminUser(config.Users, config.Views, config.Logger))))
	r.router.Handle("POST /admin/users/{id}/sessions/revoke", browserAuthn(adminAuthz(endpoint.RevokeAdminUserSessions(config.Users, config.Logger))))
	r.router.Handle("POST /admin/users/{id}/oidc/remove", browserAuthn(adminAuthz(endpoint.RemoveAdminOIDCIdentity(config.Users, config.Logger))))
	r.router.Handle("POST /admin/oidc/pending/{id}/approve", browserAuthn(adminAuthz(endpoint.ApprovePendingOIDCIdentity(config.Users, config.Logger))))
	r.router.Handle("POST /admin/oidc/pending/{id}/link", browserAuthn(adminAuthz(endpoint.LinkPendingOIDCIdentity(config.Users, config.Logger))))
	r.router.Handle("POST /admin/oidc/pending/{id}/reject", browserAuthn(adminAuthz(endpoint.RejectPendingOIDCIdentity(config.Users, config.Logger))))
	r.router.Handle("POST /admin/oidc/pending/{id}/reopen", browserAuthn(adminAuthz(endpoint.ReopenPendingOIDCIdentity(config.Users, config.Logger))))
	r.router.Handle("POST /admin/groups", browserAuthn(adminAuthz(endpoint.CreateAdminGroup(config.Groups, config.Logger))))
	r.router.Handle("POST /admin/groups/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminGroup(config.Groups, config.Logger))))
	r.router.Handle("POST /admin/navigation", browserAuthn(adminAuthz(endpoint.SaveAdminNavigationIcon(config.Navigation, config.Logger))))
	r.router.Handle("POST /admin/bin/restore/{slug...}", browserAuthn(adminAuthz(endpoint.RestoreAdminPage(config.RecycleBin, config.Logger))))
	r.router.Handle("POST /admin/bin/delete/{slug...}", browserAuthn(adminAuthz(endpoint.PermanentlyDeleteAdminPage(config.RecycleBin, config.Logger))))
	r.router.Handle("POST /admin/tags/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminTag(config.Administration, config.Logger))))
	r.router.Handle("POST /admin/tokens", browserAuthn(adminAuthz(endpoint.CreateAdminToken(config.Tokens, config.Logger))))
	r.router.Handle("DELETE /admin/tokens/{id}", browserAuthn(adminAuthz(endpoint.DeleteAdminToken(config.Tokens, config.Logger))))
	r.router.Handle(
		"POST /admin/export",
		browserAuthn(adminAuthz(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger))),
	)

	r.router.Handle("GET /api/admin/users", apiAuthn(adminAuthz(endpoint.SearchAdminUsers(config.Users, config.Logger))))
	r.router.Handle(
		"GET /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AdminGroupMembers(config.Groups, config.Logger))),
	)
	r.router.Handle(
		"POST /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AddAdminGroupMember(config.Groups, config.Users, config.Logger))),
	)
	r.router.Handle(
		"DELETE /api/admin/groups/{id}/members/{userID}",
		apiAuthn(adminAuthz(endpoint.RemoveAdminGroupMember(config.Groups, config.Logger))),
	)
	r.router.Handle(
		"POST /api/admin/export",
		apiAuthn(adminAuthz(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger))),
	)
	r.router.Handle(
		"DELETE /api/admin/bin/{slug...}",
		apiAuthn(adminAuthz(endpoint.PermanentlyDeletePage(config.RecycleBin, config.Logger))),
	)
}

// addPageRoutes registers browser page workflows, collaboration, and export endpoints.
func (r routeRegistrar) addPageRoutes() {
	config := r.config
	browserAuthn := r.router.Browser
	adminAuthz := r.router.Admin
	editorAuthz := r.router.Editor
	r.router.Handle(
		"POST /plugins/actions/{pluginID}/{moduleID}/{actionID}",
		browserAuthn(endpoint.PluginWidgetCommand(config.PageReports, config.Navigation, config.Renderer)),
	)

	r.router.Handle(
		"GET /export/markdown/{slug...}",
		browserAuthn(endpoint.ExportPageMarkdown(config.PageLookup, config.Media, config.Logger)),
	)
	r.router.Handle(
		"POST /export/plugin/{pluginID}/{moduleID}/{slug...}",
		browserAuthn(endpoint.ExportPagePlugin(config.PageReports, config.Navigation, config.Renderer, config.Logger)),
	)
	exportPDF := browserAuthn(endpoint.ExportPagePDF(
		config.PageReports,
		config.Settings,
		config.Navigation,
		config.Media,
		config.Renderer,
		config.Views,
		config.Logger,
	))
	r.router.Handle("GET /export/pdf/{slug...}", exportPDF)
	r.router.Handle("POST /export/pdf/{slug...}", exportPDF)
	r.router.Handle(
		"POST /export/preview/{slug...}",
		browserAuthn(endpoint.PreviewPageExport(
			config.PageReports,
			config.Settings,
			config.Navigation,
			config.Media,
			config.Renderer,
			config.Logger,
		)),
	)

	r.router.Handle("POST /pages/delete/{slug...}", browserAuthn(adminAuthz(endpoint.DeletePageForm(config.Pages, config.Views))))
	r.router.Handle("POST /pages/move/{slug...}", browserAuthn(editorAuthz(endpoint.MovePageForm(config.Pages, config.Logger))))
	r.router.Handle("POST /pages/review/{slug...}", browserAuthn(editorAuthz(endpoint.ReviewPageForm(config.Pages, config.Logger))))
	r.router.Handle("POST /pages/approval/request/{slug...}", browserAuthn(editorAuthz(endpoint.RequestPageReview(config.Pages, config.Logger))))
	r.router.Handle("POST /pages/approval/update/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.UpdatePageReview(config.Pages, config.Logger))))
	r.router.Handle("POST /pages/approval/cancel/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.CancelPageReview(config.Pages, config.Logger))))
	r.router.Handle("POST /pages/approval/decide/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.DecidePageReview(config.Pages, config.Logger))))
	r.router.Handle("GET /reviews/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.PageReview(config.BrowserContext, config.Pages, config.Views))))
	r.router.Handle("POST /reviews/{id}/comments/{slug...}", browserAuthn(editorAuthz(endpoint.AddPageReviewComment(config.Pages, config.Views))))
	r.router.Handle(pageReviewSuggestionApplyPattern, browserAuthn(editorAuthz(endpoint.ApplyPageReviewSuggestion(config.Pages, config.Views))))
	r.router.Handle(pageReviewSuggestionsApplyAllPattern, browserAuthn(editorAuthz(endpoint.ApplyAllPageReviewSuggestions(config.Pages, config.Views))))
	r.router.Handle("POST /page-comments/{slug...}", browserAuthn(endpoint.AddPageComment(config.Pages, config.Views)))
	r.router.Handle(pageCommentSuggestionApplyPattern, browserAuthn(editorAuthz(endpoint.ApplyPageCommentSuggestion(config.Pages, config.Views))))
	r.router.Handle("POST /page-comments/resolve/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.ResolvePageComment(config.Pages, config.Views))))

	editPage := endpoint.EditPage(config.BrowserContext, config.Editor, config.Views)
	r.router.Handle("GET /pages/new", browserAuthn(editorAuthz(editPage)))
	r.router.Handle("GET /edit/{slug...}", browserAuthn(editorAuthz(editPage)))
	r.router.Handle(
		"POST /pages",
		browserAuthn(editorAuthz(endpoint.SavePageForm(config.EditorSave, config.Views))),
	)
	r.router.Handle("POST /pages/{slug...}", browserAuthn(endpoint.FavoritePage(config.PagePersonal, config.Views)))
	r.router.Handle("POST /page-watch/{slug...}", browserAuthn(endpoint.WatchPage(config.PagePersonal, config.Views)))
	r.router.Handle("GET /revisions/{slug...}", browserAuthn(endpoint.RevisionHistory(config.PageHistory, config.Views)))
	r.router.Handle(
		"POST /revisions/{number}/restore/{slug...}",
		browserAuthn(editorAuthz(endpoint.RestoreRevision(config.Pages, config.Views))),
	)
	r.router.Handle(
		"GET /pages/{slug...}",
		browserAuthn(endpoint.ViewPage(
			config.BrowserContext,
			config.PageReports,
			config.PageRender,
			config.ViewPage,
			config.Renderer,
			config.Views,
		)),
	)
}

// addAPIRoutes registers authenticated machine-readable application endpoints.
func (r routeRegistrar) addAPIRoutes() {
	config := r.config
	apiAuthn := r.router.API
	adminAuthz := r.router.Admin
	editorAuthz := r.router.Editor

	r.router.Handle("GET /api/icons", apiAuthn(editorAuthz(endpoint.SearchIcons(config.Views.IconCatalog()))))
	r.router.Handle("GET /api/pages", apiAuthn(endpoint.ListPages(config.PageSearch, config.Logger)))
	r.router.Handle("POST /api/pages", apiAuthn(editorAuthz(endpoint.SavePage(config.Pages, config.Logger))))
	r.router.Handle(
		"POST /api/preview",
		apiAuthn(editorAuthz(endpoint.PreviewMarkdown(
			config.Navigation,
			config.PageReports,
			config.Renderer,
			config.Logger,
		))),
	)
	r.router.Handle("GET /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.GetPageDraft(config.Drafts, config.Logger))))
	r.router.Handle("PUT /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.SavePageDraft(config.Drafts, config.Logger))))
	r.router.Handle("DELETE /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.DeletePageDraft(config.Drafts, config.Logger))))
	r.router.Handle("GET /api/page-presence/{slug...}", apiAuthn(endpoint.PageEditors(config.Pages, config.Logger)))
	r.router.Handle("PUT /api/page-presence/{slug...}", apiAuthn(editorAuthz(endpoint.TouchPageEditor(config.Pages, config.Logger))))
	r.router.Handle("DELETE /api/page-presence/{slug...}", apiAuthn(editorAuthz(endpoint.LeavePageEditor(config.Pages, config.Logger))))
	r.router.Handle("GET /api/pages/{slug...}", apiAuthn(endpoint.GetPage(config.PageLookup, config.Logger)))
	r.router.Handle("PUT /api/pages/{slug...}", apiAuthn(editorAuthz(endpoint.SavePage(config.Pages, config.Logger))))
	r.router.Handle("DELETE /api/pages/{slug...}", apiAuthn(adminAuthz(endpoint.DeletePage(config.Pages, config.Logger))))
	r.router.Handle("GET /api/search", apiAuthn(endpoint.SearchAPI(config.PageSearch, config.Logger)))
	r.router.Handle("GET /api/graph", apiAuthn(endpoint.KnowledgeGraphAPI(config.Knowledge, config.Logger)))
	r.router.Handle(
		"GET /api/editor/catalog",
		apiAuthn(editorAuthz(endpoint.EditorCatalog(config.Navigation, config.PageDirectory, config.Renderer.PluginManager(), config.Logger))),
	)
	r.router.Handle("GET /api/mentions/users", apiAuthn(endpoint.MentionUsers(config.Users, config.Logger)))
	r.router.Handle("GET /api/notifications", apiAuthn(endpoint.NotificationsAPI(config.Notifications, config.Logger)))
	r.router.Handle("POST /api/notifications/{id}/read", apiAuthn(endpoint.MarkNotificationRead(config.Notifications, config.Logger)))
	r.router.Handle("GET /api/tags", apiAuthn(endpoint.Tags(config.PageSearch, config.Logger)))
	r.router.Handle("GET /api/groups", apiAuthn(endpoint.GroupsAPI(config.Groups, config.Logger)))
	r.router.Handle("GET /api/images", apiAuthn(editorAuthz(endpoint.ListImages(config.Media, config.Logger))))
	r.router.Handle("GET /api/attachments", apiAuthn(editorAuthz(endpoint.ListAttachments(config.Media, config.Logger))))
	r.router.Handle("POST /api/attachments", apiAuthn(editorAuthz(endpoint.UploadAttachment(config.Media, config.Logger))))
	r.router.Handle("DELETE /api/attachments/{id}", apiAuthn(editorAuthz(endpoint.DeleteAttachment(config.Media, config.Logger))))
	r.router.Handle("POST /api/images", apiAuthn(editorAuthz(endpoint.UploadImage(config.Media, config.Logger))))
	r.router.Handle("DELETE /api/images/{id}", apiAuthn(editorAuthz(endpoint.DeleteImage(config.Media, config.Logger))))
	r.router.Handle("GET /api/recent", apiAuthn(endpoint.Recent(config.PageSearch, config.Logger)))
}

// addFallbackRoutes registers machine-readable API and themed browser not-found handlers.
func (r routeRegistrar) addFallbackRoutes() {
	r.router.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})
	r.router.Handle("GET /", r.router.Browser(endpoint.NotFound(r.config.BrowserContext, r.config.Views)))
}
