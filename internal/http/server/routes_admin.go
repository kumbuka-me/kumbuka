package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// registerAdminRoutes registers administrator browser pages, mutations, and administrator APIs.
func registerAdminRoutes(mux *http.ServeMux, config Config) {
	browserAuthn := middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator)
	apiAuthn := middleware.AuthenticateAPI(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator)
	adminAuthz := middleware.RequireRole(domain.UserRoleAdmin)
	browserAdmin := func(handler http.Handler) http.Handler { return browserAuthn(adminAuthz(handler)) }
	apiAdmin := func(handler http.Handler) http.Handler { return apiAuthn(adminAuthz(handler)) }

	renderRebuilds := pagecontent.NewRebuilds(
		config.PageDirectory,
		config.PageReports,
		config.PageRender,
		config.Renderer,
		config.Logger,
		config.RoutePrefix,
	)

	registerAdminPluginRoutes(mux, config, browserAdmin, renderRebuilds)
	registerAdminConfigurationRoutes(mux, config, browserAdmin)
	registerAdminContentRoutes(mux, config, browserAdmin, renderRebuilds)
	registerAdminIdentityRoutes(mux, config, browserAdmin)
	registerAdminIntegrationRoutes(mux, config, browserAdmin)
	registerAdminAPIRoutes(mux, config, apiAdmin)
}

// registerAdminPluginRoutes registers plugin lifecycle and plugin-settings administration.
func registerAdminPluginRoutes(
	mux *http.ServeMux,
	config Config,
	protect middleware.Middleware,
	renderRebuilds *pagecontent.Rebuilds,
) {
	pluginManager := config.Renderer.PluginManager()
	pluginsAdmin := endpoint.NewAdminPlugins(config.PluginAdmin, config.BrowserContext, config.Views).
		WithRenderRebuilds(renderRebuilds)
	pluginSettings := endpoint.NewAdminPluginSettings(pluginManager, config.BrowserContext, config.Views)

	mux.Handle("GET /admin/plugins", protect(http.HandlerFunc(pluginsAdmin.List)))
	mux.Handle("GET /admin/plugins/{pluginID}/preview.png", protect(endpoint.PluginPreview(pluginManager)))
	mux.Handle("POST /admin/plugins", protect(http.HandlerFunc(pluginsAdmin.Install)))
	mux.Handle("POST /admin/plugins/check-updates", protect(http.HandlerFunc(pluginsAdmin.CheckUpdates)))
	mux.Handle("POST /admin/plugins/{pluginID}/{action}", protect(http.HandlerFunc(pluginsAdmin.Action)))
	mux.Handle("GET /admin/plugin-settings/{pluginID}", protect(http.HandlerFunc(pluginSettings.Show)))
	mux.Handle("POST /admin/plugin-settings/{pluginID}/{action}", protect(http.HandlerFunc(pluginSettings.Action)))
}

// registerAdminConfigurationRoutes registers application configuration and appearance administration.
func registerAdminConfigurationRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	pluginManager := config.Renderer.PluginManager()
	mux.Handle("GET /admin", protect(endpoint.Administration(config.BrowserContext, config.Administration, config.System, config.Views)))
	mux.Handle("GET /admin/configuration", protect(endpoint.AdminConfiguration(config.BrowserContext, config.Groups, config.Users, config.Settings, config.Views)))
	mux.Handle("GET /admin/editor-toolbar", protect(endpoint.AdminEditorToolbar(config.BrowserContext, config.Views)))
	mux.Handle("GET /admin/branding", protect(endpoint.AdminBranding(config.BrowserContext, config.Views)))
	mux.Handle("GET /admin/health", protect(endpoint.AdminDocumentationHealth(config.BrowserContext, config.Administration, config.Views)))
	mux.Handle("GET /admin/audit", protect(endpoint.AdminAudit(config.BrowserContext, config.Administration, config.Views)))
	mux.Handle("POST /admin/settings", protect(endpoint.SaveAdminSettings(config.Settings, config.Views, config.Logger)))
	mux.Handle("POST /admin/editor-toolbar", protect(endpoint.SaveAdminEditorToolbar(config.Settings, pluginManager, config.Views)))
	mux.Handle("POST /admin/branding/logo", protect(endpoint.SaveAdminBrandLogo(config.Settings, config.Logger)))
	mux.Handle("POST /admin/branding/logo/reset", protect(endpoint.ResetAdminBrandLogo(config.Settings, config.Logger)))
	mux.Handle("POST /admin/pdf", protect(endpoint.SaveAdminPDFSettings(config.Settings, config.Logger)))
	mux.Handle("POST /admin/pdf/test", protect(endpoint.TestAdminPDFService(config.Settings, config.Logger)))
	mux.Handle("POST /admin/pdf/headers/{id}/reveal", protect(endpoint.RevealAdminPDFHeader(config.Settings, config.Logger)))
	mux.Handle("POST /admin/authentication", protect(endpoint.SaveAdminAuthentication(config.Settings, config.BrowserAuth, config.Views)))
}

// registerAdminContentRoutes registers page, template, media, navigation, and archive administration.
func registerAdminContentRoutes(
	mux *http.ServeMux,
	config Config,
	protect middleware.Middleware,
	renderRebuilds *pagecontent.Rebuilds,
) {
	mux.Handle("GET /admin/templates", protect(endpoint.AdminPageTemplates(config.BrowserContext, config.Templates, config.Groups, config.Views)))
	mux.Handle("GET /admin/permissions", protect(endpoint.AdminPageAccess(config.BrowserContext, config.Access, config.Groups, config.Navigation, config.Views)))
	mux.Handle("POST /admin/permissions", protect(endpoint.SaveAdminPageAccess(config.Access, config.Logger)))
	mux.Handle("POST /admin/permissions/{id}/delete", protect(endpoint.DeleteAdminPageAccess(config.Access, config.Logger)))
	mux.Handle("GET /admin/pages", protect(endpoint.AdminPages(config.BrowserContext, config.PageDirectory, config.Groups, config.Views)))
	mux.Handle("POST /admin/pages/bulk", protect(endpoint.BulkAdminPages(config.PageBulk, config.PageLookup, config.Media, config.Logger)))
	mux.Handle("POST /admin/pages/render-all", protect(endpoint.QueueAllAdminPageRenders(renderRebuilds, config.Logger)))
	mux.Handle("POST /admin/pages/render-pending", protect(endpoint.FlushPendingAdminPageRenders(renderRebuilds)))
	mux.Handle("POST /admin/pages/render/{slug...}", protect(endpoint.RebuildAdminPageRender(renderRebuilds, config.Logger)))
	mux.Handle("GET /admin/import", protect(endpoint.AdminImport(config.BrowserContext, config.Views)))
	mux.Handle("POST /admin/import", protect(endpoint.ImportPagesWithPortableArchive(config.PageBulk, config.PortableImport, config.Logger)))
	mux.Handle("POST /admin/templates", protect(endpoint.CreateAdminPageTemplate(config.Templates, config.Logger)))
	mux.Handle("POST /admin/templates/{id}", protect(endpoint.UpdateAdminPageTemplate(config.Templates, config.Logger)))
	mux.Handle("POST /admin/templates/{id}/delete", protect(endpoint.DeleteAdminPageTemplate(config.Templates, config.Logger)))
	mux.Handle("GET /admin/navigation", protect(endpoint.AdminNavigation(config.BrowserContext, config.Navigation, config.Views)))
	mux.Handle("POST /admin/navigation", protect(endpoint.SaveAdminNavigationIcon(config.Navigation, config.Logger)))
	mux.Handle("GET /admin/bin", protect(endpoint.AdminBin(config.BrowserContext, config.RecycleBin, config.Views)))
	mux.Handle("POST /admin/bin/restore/{slug...}", protect(endpoint.RestoreAdminPage(config.RecycleBin, config.Logger)))
	mux.Handle("POST /admin/bin/delete/{slug...}", protect(endpoint.PermanentlyDeleteAdminPage(config.RecycleBin, config.Logger)))
	mux.Handle("GET /admin/tags", protect(endpoint.AdminTags(config.BrowserContext, config.Administration, config.Views)))
	mux.Handle("POST /admin/tags/{id}/delete", protect(endpoint.DeleteAdminTag(config.Administration, config.Logger)))
	mux.Handle("GET /admin/exports", protect(endpoint.AdminExports(config.BrowserContext, config.Navigation, config.Views)))
	mux.Handle("POST /admin/export", protect(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger)))
	mux.Handle("GET /admin/images", protect(endpoint.AdminImages(config.BrowserContext, config.Media, config.Views)))
	mux.Handle("GET /admin/attachments", protect(endpoint.AdminAttachments(config.BrowserContext, config.Media, config.Views)))
	mux.Handle("POST /admin/attachments/{id}/delete", protect(endpoint.DeleteAdminAttachment(config.Media, config.Views)))
}

// registerAdminIdentityRoutes registers user, group, token, and external-identity administration.
func registerAdminIdentityRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	mux.Handle("GET /admin/users", protect(endpoint.AdminUsers(config.BrowserContext, config.Users, config.Groups, config.Views)))
	mux.Handle("GET /admin/groups", protect(endpoint.AdminGroups(config.BrowserContext, config.Groups, config.Views)))
	mux.Handle("GET /admin/tokens", protect(endpoint.AdminTokens(config.BrowserContext, config.Users, config.Tokens, config.Views)))
	mux.Handle("POST /admin/users/{id}", protect(endpoint.UpdateAdminUser(config.Users, config.Views, config.Logger)))
	mux.Handle("POST /admin/users/{id}/trusted-proxy/relink", protect(endpoint.RelinkAdminTrustedProxyIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/users/{id}/sessions/revoke", protect(endpoint.RevokeAdminUserSessions(config.Users, config.Logger)))
	mux.Handle("POST /admin/users/{id}/oidc/remove", protect(endpoint.RemoveAdminOIDCIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/oidc/pending/{id}/approve", protect(endpoint.ApprovePendingOIDCIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/oidc/pending/{id}/link", protect(endpoint.LinkPendingOIDCIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/oidc/pending/{id}/reject", protect(endpoint.RejectPendingOIDCIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/oidc/pending/{id}/reopen", protect(endpoint.ReopenPendingOIDCIdentity(config.Users, config.Logger)))
	mux.Handle("POST /admin/groups", protect(endpoint.CreateAdminGroup(config.Groups, config.Logger)))
	mux.Handle("POST /admin/groups/{id}/delete", protect(endpoint.DeleteAdminGroup(config.Groups, config.Logger)))
	mux.Handle("POST /admin/tokens", protect(endpoint.CreateAdminToken(config.Tokens, config.Logger)))
	mux.Handle("DELETE /admin/tokens/{id}", protect(endpoint.DeleteAdminToken(config.Tokens, config.Logger)))
}

// registerAdminIntegrationRoutes registers webhook administration.
func registerAdminIntegrationRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	mux.Handle("GET /admin/webhooks", protect(endpoint.AdminWebhooks(config.BrowserContext, config.Webhooks, config.Views)))
	mux.Handle("POST /admin/webhooks", protect(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger)))
	mux.Handle("POST /admin/webhooks/{id}", protect(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger)))
	mux.Handle("POST /admin/webhooks/{id}/delete", protect(endpoint.DeleteAdminWebhook(config.Webhooks, config.Logger)))
	mux.Handle("POST /admin/webhooks/{id}/test", protect(endpoint.TestAdminWebhook(config.Webhooks, config.Logger)))
	mux.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", protect(endpoint.RevealAdminWebhookHeader(config.Webhooks, config.Logger)))
}

// registerAdminAPIRoutes registers administrator-only JSON API endpoints.
func registerAdminAPIRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	mux.Handle("GET /api/admin/users", protect(endpoint.SearchAdminUsers(config.Users, config.Logger)))
	mux.Handle("GET /api/admin/groups/{id}/members", protect(endpoint.AdminGroupMembers(config.Groups, config.Logger)))
	mux.Handle("POST /api/admin/groups/{id}/members", protect(endpoint.AddAdminGroupMember(config.Groups, config.Users, config.Logger)))
	mux.Handle("DELETE /api/admin/groups/{id}/members/{userID}", protect(endpoint.RemoveAdminGroupMember(config.Groups, config.Logger)))
	mux.Handle("POST /api/admin/export", protect(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger)))
	mux.Handle("DELETE /api/admin/bin/{slug...}", protect(endpoint.PermanentlyDeletePage(config.RecycleBin, config.Logger)))
}
