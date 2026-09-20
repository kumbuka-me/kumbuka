package routes

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

const (
	pageCommentSuggestionApplyPattern    = "POST /page-comments/suggestions/apply/{id}/{slug...}"
	pageReviewSuggestionApplyPattern     = "POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}"
	pageReviewSuggestionsApplyAllPattern = "POST /reviews/{id}/suggestions/apply-all/{slug...}"
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

	r.mux.HandleFunc("GET /plugins/styles.css", endpoint.PluginPresentationStyles(renderer.PluginManager()))
	r.mux.HandleFunc("GET /plugins/runtime.js", endpoint.PluginBrowserRuntime(config.Assets))
	r.mux.HandleFunc("GET /plugins/{pluginID}/{digest}/assets/{asset...}", endpoint.PluginAssets(renderer.PluginManager()))
	r.mux.HandleFunc("GET /plugins/{pluginID}/{digest}/frames/{frame}", endpoint.PluginFrame(renderer.PluginManager()))
	r.mux.HandleFunc("GET /healthz", endpoint.Health(config.System))
	r.mux.Handle("GET /robots.txt", endpoint.Robots(config.Settings, config.Views, config.Logger))
	r.mux.Handle("GET /sitemap.xml", endpoint.Sitemap(config.Settings, config.Catalog, config.Access, config.Views, config.Logger))
	r.mux.Handle("GET /assets/", endpoint.Assets(config.Assets))
	r.mux.Handle("GET /sw.js", endpoint.ServiceWorker(config.Assets))
	r.mux.Handle("GET /brand/logo", endpoint.BrandLogo(config.Settings, config.Assets, config.Logger))
	r.mux.Handle("GET /auth/login", browserAuth.Login)

	localLogin := endpoint.LocalLogin(config.Settings, config.System, browserAuth, config.Views)
	r.mux.Handle("GET /auth/local", localLogin)
	r.mux.Handle("POST /auth/local", localLogin)

	setup := endpoint.Setup(config.Settings, config.System, browserAuth, config.Views)
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
	r.mux.Handle("GET /{$}", browserAuthn(endpoint.Home(config.ViewData, config.Catalog, config.Drafts, config.Access, config.Renderer, config.Views)))
	r.mux.Handle("GET /search", browserAuthn(endpoint.Search(config.ViewData, config.Catalog, config.Access, config.Views)))
	r.mux.Handle("GET /graph", browserAuthn(endpoint.KnowledgeGraphPage(config.ViewData, config.Views)))
	r.mux.Handle("GET /p/{id}", browserAuthn(endpoint.PagePermalink(config.Catalog, config.Logger)))
	r.mux.Handle(
		"GET /settings",
		browserAuthn(endpoint.Settings(config.ViewData, config.Users, config.Tokens, config.Media, config.BrowserAuth.Local, config.Views)),
	)

	r.mux.Handle("GET /media/{id}/{name...}", mediaAuthn(endpoint.ServeImage(config.Media, config.Logger)))
	r.mux.Handle("GET /attachments/{id}/{name...}", mediaAuthn(endpoint.ServeAttachment(config.Media, config.Logger)))

	r.mux.Handle("POST /settings/preferences", browserAuthn(endpoint.SavePreferences(config.Preferences, config.Renderer.PluginManager(), config.Views)))
	r.mux.Handle("POST /settings/local-password", browserAuthn(endpoint.ChangeLocalPassword(config.BrowserAuth.Local, config.Logger)))
	r.mux.Handle(
		"POST /settings/preferences/page-contents",
		browserAuthn(endpoint.SavePageContentsPreference(config.Preferences, config.Views)),
	)
	r.mux.Handle(
		"POST /settings/preferences/navigation-state",
		browserAuthn(endpoint.SaveNavigationState(config.Preferences, config.Logger)),
	)
	r.mux.Handle(
		"POST /settings/preferences/sidebar-width",
		browserAuthn(endpoint.SaveSidebarWidth(config.Preferences, config.Logger)),
	)
	r.mux.Handle("POST /settings/saved-searches", browserAuthn(endpoint.CreateSavedSearch(config.Knowledge, config.Logger)))
	r.mux.Handle(
		"POST /settings/saved-searches/{id}/delete",
		browserAuthn(endpoint.DeleteSavedSearch(config.Knowledge, config.Logger)),
	)
	r.mux.Handle("POST /settings/tokens", browserAuthn(endpoint.CreatePersonalToken(config.Tokens, config.Logger)))
	r.mux.Handle("DELETE /settings/tokens/{id}", browserAuthn(endpoint.DeletePersonalToken(config.Tokens, config.Logger)))
	r.mux.Handle("POST /notifications/{id}/open", browserAuthn(endpoint.OpenNotification(config.Notifications, config.Logger)))
}

// addAdminRoutes registers administrator pages, mutations, and administrator APIs.
func (r routeRegistrar) addAdminRoutes() {
	config := r.config
	browserAuthn := r.policies.browserAuthn
	apiAuthn := r.policies.apiAuthn
	adminAuthz := r.policies.adminAuthz

	pluginManager := config.Renderer.PluginManager()
	pluginsAdmin := endpoint.NewAdminPlugins(pluginManager, config.PluginUpdates, config.ViewData, config.Views)
	r.mux.Handle("GET /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.List))))
	r.mux.Handle("POST /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Install))))
	r.mux.Handle("POST /admin/plugins/check-updates", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.CheckUpdates))))
	r.mux.Handle("POST /admin/plugins/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Action))))
	pluginSettings := endpoint.NewAdminPluginSettings(pluginManager, config.ViewData, config.Views)
	r.mux.Handle("GET /admin/plugin-settings/{pluginID}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Show))))
	r.mux.Handle("POST /admin/plugin-settings/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Action))))

	r.mux.Handle("GET /admin", browserAuthn(adminAuthz(endpoint.Administration(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle(
		"GET /admin/configuration",
		browserAuthn(adminAuthz(endpoint.AdminConfiguration(config.ViewData, config.Groups, config.Users, config.Settings, config.Views))),
	)
	r.mux.Handle(
		"GET /admin/branding",
		browserAuthn(adminAuthz(endpoint.AdminBranding(config.ViewData, config.Views))),
	)
	r.mux.Handle(
		"GET /admin/health",
		browserAuthn(adminAuthz(endpoint.AdminDocumentationHealth(config.ViewData, config.Administration, config.Views))),
	)
	r.mux.Handle(
		"GET /admin/templates",
		browserAuthn(adminAuthz(endpoint.AdminPageTemplates(config.ViewData, config.Templates, config.Groups, config.Views))),
	)
	r.mux.Handle("GET /admin/permissions", browserAuthn(adminAuthz(endpoint.AdminPageAccess(config.ViewData, config.Access, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/webhooks", browserAuthn(adminAuthz(endpoint.AdminWebhooks(config.ViewData, config.Webhooks, config.Views))))
	r.mux.Handle("POST /admin/webhooks", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/test", browserAuthn(adminAuthz(endpoint.TestAdminWebhook(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminWebhookHeader(config.Webhooks, config.Logger))))
	r.mux.Handle("POST /admin/permissions", browserAuthn(adminAuthz(endpoint.SaveAdminPageAccess(config.Access, config.Logger))))
	r.mux.Handle("POST /admin/permissions/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageAccess(config.Access, config.Logger))))
	r.mux.Handle("GET /admin/audit", browserAuthn(adminAuthz(endpoint.AdminAudit(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle("GET /admin/pages", browserAuthn(adminAuthz(endpoint.AdminPages(config.ViewData, config.Catalog, config.Groups, config.Views))))
	r.mux.Handle(
		"POST /admin/pages/bulk",
		browserAuthn(adminAuthz(endpoint.BulkAdminPages(config.Pages, config.Catalog, config.Media, config.Logger))),
	)
	r.mux.Handle("GET /admin/import", browserAuthn(adminAuthz(endpoint.AdminImport(config.ViewData, config.Views))))
	r.mux.Handle(
		"POST /admin/import",
		browserAuthn(adminAuthz(endpoint.ImportPagesWithPortableArchive(config.Pages, config.Media, config.Groups, config.Logger))),
	)
	r.mux.Handle("POST /admin/templates", browserAuthn(adminAuthz(endpoint.CreateAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("POST /admin/templates/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("POST /admin/templates/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageTemplate(config.Templates, config.Logger))))
	r.mux.Handle("GET /admin/users", browserAuthn(adminAuthz(endpoint.AdminUsers(config.ViewData, config.Users, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/groups", browserAuthn(adminAuthz(endpoint.AdminGroups(config.ViewData, config.Groups, config.Views))))
	r.mux.Handle("GET /admin/navigation", browserAuthn(adminAuthz(endpoint.AdminNavigation(config.ViewData, config.Navigation, config.Views))))
	r.mux.Handle("GET /admin/bin", browserAuthn(adminAuthz(endpoint.AdminBin(config.ViewData, config.RecycleBin, config.Views))))
	r.mux.Handle("GET /admin/tags", browserAuthn(adminAuthz(endpoint.AdminTags(config.ViewData, config.Administration, config.Views))))
	r.mux.Handle("GET /admin/tokens", browserAuthn(adminAuthz(endpoint.AdminTokens(config.ViewData, config.Users, config.Tokens, config.Views))))
	r.mux.Handle("GET /admin/exports", browserAuthn(adminAuthz(endpoint.AdminExports(config.ViewData, config.Navigation, config.Views))))
	r.mux.Handle("GET /admin/images", browserAuthn(adminAuthz(endpoint.AdminImages(config.ViewData, config.Media, config.Views))))
	r.mux.Handle("POST /admin/settings", browserAuthn(adminAuthz(endpoint.SaveAdminSettings(config.Settings, config.Views, config.Logger))))
	r.mux.Handle(
		"POST /admin/branding/logo",
		browserAuthn(adminAuthz(endpoint.SaveAdminBrandLogo(config.Settings, config.Logger))),
	)
	r.mux.Handle(
		"POST /admin/branding/logo/reset",
		browserAuthn(adminAuthz(endpoint.ResetAdminBrandLogo(config.Settings, config.Logger))),
	)
	r.mux.Handle("POST /admin/pdf", browserAuthn(adminAuthz(endpoint.SaveAdminPDFSettings(config.Settings, config.Logger))))
	r.mux.Handle("POST /admin/pdf/test", browserAuthn(adminAuthz(endpoint.TestAdminPDFService(config.Settings, config.Logger))))
	r.mux.Handle("POST /admin/pdf/headers/{id}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminPDFHeader(config.Settings, config.Logger))))
	r.mux.Handle(
		"POST /admin/authentication",
		browserAuthn(adminAuthz(endpoint.SaveAdminAuthentication(config.Settings, config.BrowserAuth, config.Views))),
	)

	r.mux.Handle("POST /admin/users/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminUser(config.Users, config.Views, config.Logger))))
	r.mux.Handle("POST /admin/users/{id}/sessions/revoke", browserAuthn(adminAuthz(endpoint.RevokeAdminUserSessions(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/users/{id}/oidc/remove", browserAuthn(adminAuthz(endpoint.RemoveAdminOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/approve", browserAuthn(adminAuthz(endpoint.ApprovePendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/link", browserAuthn(adminAuthz(endpoint.LinkPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/reject", browserAuthn(adminAuthz(endpoint.RejectPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/oidc/pending/{id}/reopen", browserAuthn(adminAuthz(endpoint.ReopenPendingOIDCIdentity(config.Users, config.Logger))))
	r.mux.Handle("POST /admin/groups", browserAuthn(adminAuthz(endpoint.CreateAdminGroup(config.Groups, config.Logger))))
	r.mux.Handle("POST /admin/groups/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminGroup(config.Groups, config.Logger))))
	r.mux.Handle("POST /admin/navigation", browserAuthn(adminAuthz(endpoint.SaveAdminNavigationIcon(config.Navigation, config.Logger))))
	r.mux.Handle("POST /admin/bin/restore/{slug...}", browserAuthn(adminAuthz(endpoint.RestoreAdminPage(config.RecycleBin, config.Logger))))
	r.mux.Handle("POST /admin/bin/delete/{slug...}", browserAuthn(adminAuthz(endpoint.PermanentlyDeleteAdminPage(config.RecycleBin, config.Logger))))
	r.mux.Handle("POST /admin/tags/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminTag(config.Administration, config.Logger))))
	r.mux.Handle("POST /admin/tokens", browserAuthn(adminAuthz(endpoint.CreateAdminToken(config.Tokens, config.Logger))))
	r.mux.Handle("DELETE /admin/tokens/{id}", browserAuthn(adminAuthz(endpoint.DeleteAdminToken(config.Tokens, config.Logger))))
	r.mux.Handle(
		"POST /admin/export",
		browserAuthn(adminAuthz(endpoint.ExportPortablePages(config.Catalog, config.Navigation, config.Media, config.Logger))),
	)

	r.mux.Handle("GET /api/admin/users", apiAuthn(adminAuthz(endpoint.SearchAdminUsers(config.Users, config.Logger))))
	r.mux.Handle(
		"GET /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AdminGroupMembers(config.Groups, config.Logger))),
	)
	r.mux.Handle(
		"POST /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AddAdminGroupMember(config.Groups, config.Users, config.Logger))),
	)
	r.mux.Handle(
		"DELETE /api/admin/groups/{id}/members/{userID}",
		apiAuthn(adminAuthz(endpoint.RemoveAdminGroupMember(config.Groups, config.Logger))),
	)
	r.mux.Handle(
		"POST /api/admin/export",
		apiAuthn(adminAuthz(endpoint.ExportPortablePages(config.Catalog, config.Navigation, config.Media, config.Logger))),
	)
	r.mux.Handle(
		"DELETE /api/admin/bin/{slug...}",
		apiAuthn(adminAuthz(endpoint.PermanentlyDeletePage(config.RecycleBin, config.Logger))),
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
		browserAuthn(endpoint.PluginWidgetCommand(config.Catalog, config.Navigation, config.Access, config.Renderer)),
	)

	r.mux.Handle(
		"GET /export/markdown/{slug...}",
		browserAuthn(r.pageViewAuthz(endpoint.ExportPageMarkdown(config.Catalog, config.Media, config.Logger))),
	)
	r.mux.Handle(
		"POST /export/plugin/{pluginID}/{moduleID}/{slug...}",
		browserAuthn(r.pageViewAuthz(endpoint.ExportPagePlugin(config.Catalog, config.Navigation, config.Access, config.Renderer, config.Logger))),
	)
	exportPDF := browserAuthn(r.pageViewAuthz(endpoint.ExportPagePDF(
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
		browserAuthn(r.pageViewAuthz(endpoint.PreviewPageExport(
			config.Catalog,
			config.Settings,
			config.Navigation,
			config.Media,
			config.Access,
			config.Renderer,
			config.Logger,
		))),
	)

	r.mux.Handle("POST /pages/delete/{slug...}", browserAuthn(adminAuthz(endpoint.DeletePageForm(config.Pages, config.Views))))
	r.mux.Handle("POST /pages/move/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.MovePageForm(config.Pages, config.Access, config.Logger)))))
	r.mux.Handle("POST /pages/review/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.ReviewPageForm(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/request/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.RequestPageReview(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/update/{id}/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.UpdatePageReview(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/cancel/{id}/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.CancelPageReview(config.Pages, config.Logger)))))
	r.mux.Handle("POST /pages/approval/decide/{id}/{slug...}", browserAuthn(editorAuthz(r.pageViewAuthz(endpoint.DecidePageReview(config.Pages, config.Logger)))))
	r.mux.Handle("GET /reviews/{id}/{slug...}", browserAuthn(editorAuthz(r.pageViewAuthz(endpoint.PageReview(config.ViewData, config.Pages, config.Views)))))
	r.mux.Handle("POST /reviews/{id}/comments/{slug...}", browserAuthn(editorAuthz(r.pageViewAuthz(endpoint.AddPageReviewComment(config.Pages, config.Views)))))
	r.mux.Handle(pageReviewSuggestionApplyPattern, browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.ApplyPageReviewSuggestion(config.Pages, config.Views)))))
	r.mux.Handle(pageReviewSuggestionsApplyAllPattern, browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.ApplyAllPageReviewSuggestions(config.Pages, config.Views)))))
	r.mux.Handle("POST /page-comments/{slug...}", browserAuthn(r.pageViewAuthz(endpoint.AddPageComment(config.Pages, config.Views))))
	r.mux.Handle(pageCommentSuggestionApplyPattern, browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.ApplyPageCommentSuggestion(config.Pages, config.Views)))))
	r.mux.Handle("POST /page-comments/resolve/{id}/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.ResolvePageComment(config.Pages, config.Views)))))

	editPage := endpoint.EditPage(config.ViewData, config.Catalog, config.Groups, config.Templates, config.Access, config.Views)
	r.mux.Handle("GET /pages/new", browserAuthn(editorAuthz(r.pageEditAuthz(editPage))))
	r.mux.Handle("GET /edit/{slug...}", browserAuthn(editorAuthz(r.pageEditAuthz(editPage))))
	r.mux.Handle(
		"POST /pages",
		browserAuthn(editorAuthz(endpoint.SavePageForm(config.Pages, config.Drafts, config.Templates, config.Access, config.Views))),
	)
	r.mux.Handle("POST /pages/{slug...}", browserAuthn(r.pageViewAuthz(endpoint.FavoritePage(config.Catalog, config.Views))))
	r.mux.Handle("POST /page-watch/{slug...}", browserAuthn(r.pageViewAuthz(endpoint.WatchPage(config.Catalog, config.Views))))
	r.mux.Handle("GET /revisions/{slug...}", browserAuthn(r.pageViewAuthz(endpoint.RevisionHistory(config.Catalog, config.Views))))
	r.mux.Handle(
		"POST /revisions/{number}/restore/{slug...}",
		browserAuthn(editorAuthz(r.pageEditAuthz(endpoint.RestoreRevision(config.Pages, config.Views)))),
	)
	r.mux.Handle(
		"GET /pages/{slug...}",
		browserAuthn(r.pageViewAuthz(endpoint.ViewPage(
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

	r.mux.Handle("GET /api/icons", apiAuthn(editorAuthz(endpoint.SearchIcons(config.Views.IconCatalog()))))
	r.mux.Handle("GET /api/pages", apiAuthn(endpoint.ListPages(config.Catalog, config.Access, config.Logger)))
	r.mux.Handle("POST /api/pages", apiAuthn(editorAuthz(endpoint.SavePage(config.Pages, config.Access, config.Logger))))
	r.mux.Handle(
		"POST /api/preview",
		apiAuthn(editorAuthz(endpoint.PreviewMarkdown(
			config.Navigation,
			config.Catalog,
			config.Access,
			config.Renderer,
			config.Logger,
		))),
	)
	r.mux.Handle("GET /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.GetPageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("PUT /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.SavePageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("DELETE /api/drafts/{key}", apiAuthn(editorAuthz(endpoint.DeletePageDraft(config.Drafts, config.Logger))))
	r.mux.Handle("GET /api/page-presence/{slug...}", apiAuthn(r.pageViewAuthz(endpoint.PageEditors(config.Pages, config.Logger))))
	r.mux.Handle("PUT /api/page-presence/{slug...}", apiAuthn(editorAuthz(r.pageEditAuthz(endpoint.TouchPageEditor(config.Pages, config.Logger)))))
	r.mux.Handle("DELETE /api/page-presence/{slug...}", apiAuthn(editorAuthz(r.pageEditAuthz(endpoint.LeavePageEditor(config.Pages, config.Logger)))))
	r.mux.Handle("GET /api/pages/{slug...}", apiAuthn(r.pageViewAuthz(endpoint.GetPage(config.Catalog, config.Logger))))
	r.mux.Handle("PUT /api/pages/{slug...}", apiAuthn(editorAuthz(endpoint.SavePage(config.Pages, config.Access, config.Logger))))
	r.mux.Handle("DELETE /api/pages/{slug...}", apiAuthn(adminAuthz(r.pageEditAuthz(endpoint.DeletePage(config.Pages, config.Logger)))))
	r.mux.Handle("GET /api/search", apiAuthn(endpoint.SearchAPI(config.Catalog, config.Access, config.Logger)))
	r.mux.Handle("GET /api/graph", apiAuthn(endpoint.KnowledgeGraphAPI(config.Knowledge, config.Access, config.Logger)))
	r.mux.Handle(
		"GET /api/editor/catalog",
		apiAuthn(editorAuthz(endpoint.EditorCatalog(config.Navigation, config.Catalog, config.Renderer.PluginManager(), config.Logger))),
	)
	r.mux.Handle("GET /api/mentions/users", apiAuthn(endpoint.MentionUsers(config.Users, config.Logger)))
	r.mux.Handle("GET /api/notifications", apiAuthn(endpoint.NotificationsAPI(config.Notifications, config.Logger)))
	r.mux.Handle("POST /api/notifications/{id}/read", apiAuthn(endpoint.MarkNotificationRead(config.Notifications, config.Logger)))
	r.mux.Handle("GET /api/tags", apiAuthn(endpoint.Tags(config.Catalog, config.Logger)))
	r.mux.Handle("GET /api/groups", apiAuthn(endpoint.GroupsAPI(config.Groups, config.Logger)))
	r.mux.Handle("GET /api/images", apiAuthn(editorAuthz(endpoint.ListImages(config.Media, config.Logger))))
	r.mux.Handle("GET /api/attachments", apiAuthn(editorAuthz(endpoint.ListAttachments(config.Media, config.Logger))))
	r.mux.Handle("POST /api/attachments", apiAuthn(editorAuthz(endpoint.UploadAttachment(config.Media, config.Logger))))
	r.mux.Handle("DELETE /api/attachments/{id}", apiAuthn(editorAuthz(endpoint.DeleteAttachment(config.Media, config.Logger))))
	r.mux.Handle("POST /api/images", apiAuthn(editorAuthz(endpoint.UploadImage(config.Media, config.Logger))))
	r.mux.Handle("DELETE /api/images/{id}", apiAuthn(editorAuthz(endpoint.DeleteImage(config.Media, config.Logger))))
	r.mux.Handle("GET /api/recent", apiAuthn(endpoint.Recent(config.Catalog, config.Access, config.Logger)))
}

// addFallbackRoutes registers machine-readable API and themed browser not-found handlers.
func (r routeRegistrar) addFallbackRoutes() {
	r.mux.HandleFunc("GET /api/", func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})
	r.mux.Handle("GET /", r.policies.browserAuthn(endpoint.NotFound(r.config.ViewData, r.config.Views)))
}
