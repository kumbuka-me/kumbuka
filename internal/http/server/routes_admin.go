package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// registerAdminRoutes registers administrator browser pages, mutations, and administrator APIs.
func registerAdminRoutes(mux *http.ServeMux, config Config) {
	browserAuthn := middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator)
	apiAuthn := middleware.AuthenticateAPI(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator)
	adminAuthz := middleware.RequireRole(domain.UserRoleAdmin)

	pluginManager := config.Renderer.PluginManager()
	pluginsAdmin := endpoint.NewAdminPlugins(config.PluginAdmin, config.BrowserContext, config.Views)
	mux.Handle("GET /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.List))))
	mux.Handle("GET /admin/plugins/{pluginID}/preview.png", browserAuthn(adminAuthz(endpoint.PluginPreview(pluginManager))))
	mux.Handle("POST /admin/plugins", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Install))))
	mux.Handle("POST /admin/plugins/check-updates", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.CheckUpdates))))
	mux.Handle("POST /admin/plugins/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginsAdmin.Action))))
	pluginSettings := endpoint.NewAdminPluginSettings(pluginManager, config.BrowserContext, config.Views)
	mux.Handle("GET /admin/plugin-settings/{pluginID}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Show))))
	mux.Handle("POST /admin/plugin-settings/{pluginID}/{action}", browserAuthn(adminAuthz(http.HandlerFunc(pluginSettings.Action))))

	mux.Handle("GET /admin", browserAuthn(adminAuthz(endpoint.Administration(config.BrowserContext, config.Administration, config.System, config.Views))))
	mux.Handle(
		"GET /admin/configuration",
		browserAuthn(adminAuthz(endpoint.AdminConfiguration(config.BrowserContext, config.Groups, config.Users, config.Settings, config.Views))),
	)
	mux.Handle(
		"GET /admin/editor-toolbar",
		browserAuthn(adminAuthz(endpoint.AdminEditorToolbar(config.BrowserContext, config.Views))),
	)
	mux.Handle(
		"GET /admin/branding",
		browserAuthn(adminAuthz(endpoint.AdminBranding(config.BrowserContext, config.Views))),
	)
	mux.Handle(
		"GET /admin/health",
		browserAuthn(adminAuthz(endpoint.AdminDocumentationHealth(config.BrowserContext, config.Administration, config.Views))),
	)
	mux.Handle(
		"GET /admin/templates",
		browserAuthn(adminAuthz(endpoint.AdminPageTemplates(config.BrowserContext, config.Templates, config.Groups, config.Views))),
	)
	mux.Handle("GET /admin/permissions", browserAuthn(adminAuthz(endpoint.AdminPageAccess(config.BrowserContext, config.Access, config.Groups, config.Views))))
	mux.Handle("GET /admin/webhooks", browserAuthn(adminAuthz(endpoint.AdminWebhooks(config.BrowserContext, config.Webhooks, config.Views))))
	mux.Handle("POST /admin/webhooks", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	mux.Handle("POST /admin/webhooks/{id}", browserAuthn(adminAuthz(endpoint.SaveAdminWebhook(config.Webhooks, config.Logger))))
	mux.Handle("POST /admin/webhooks/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminWebhook(config.Webhooks, config.Logger))))
	mux.Handle("POST /admin/webhooks/{id}/test", browserAuthn(adminAuthz(endpoint.TestAdminWebhook(config.Webhooks, config.Logger))))
	mux.Handle("POST /admin/webhooks/{id}/headers/{headerID}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminWebhookHeader(config.Webhooks, config.Logger))))
	mux.Handle("POST /admin/permissions", browserAuthn(adminAuthz(endpoint.SaveAdminPageAccess(config.Access, config.Logger))))
	mux.Handle("POST /admin/permissions/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageAccess(config.Access, config.Logger))))
	mux.Handle("GET /admin/audit", browserAuthn(adminAuthz(endpoint.AdminAudit(config.BrowserContext, config.Administration, config.Views))))
	mux.Handle("GET /admin/pages", browserAuthn(adminAuthz(endpoint.AdminPages(config.BrowserContext, config.PageDirectory, config.Groups, config.Views))))
	mux.Handle(
		"POST /admin/pages/bulk",
		browserAuthn(adminAuthz(endpoint.BulkAdminPages(config.PageBulk, config.PageLookup, config.Media, config.Logger))),
	)
	mux.Handle("GET /admin/import", browserAuthn(adminAuthz(endpoint.AdminImport(config.BrowserContext, config.Views))))
	mux.Handle(
		"POST /admin/import",
		browserAuthn(adminAuthz(endpoint.ImportPagesWithPortableArchive(config.PageBulk, config.Media, config.Groups, config.Logger))),
	)
	mux.Handle("POST /admin/templates", browserAuthn(adminAuthz(endpoint.CreateAdminPageTemplate(config.Templates, config.Logger))))
	mux.Handle("POST /admin/templates/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminPageTemplate(config.Templates, config.Logger))))
	mux.Handle("POST /admin/templates/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminPageTemplate(config.Templates, config.Logger))))
	mux.Handle("GET /admin/users", browserAuthn(adminAuthz(endpoint.AdminUsers(config.BrowserContext, config.Users, config.Groups, config.Views))))
	mux.Handle("GET /admin/groups", browserAuthn(adminAuthz(endpoint.AdminGroups(config.BrowserContext, config.Groups, config.Views))))
	mux.Handle("GET /admin/navigation", browserAuthn(adminAuthz(endpoint.AdminNavigation(config.BrowserContext, config.Navigation, config.Views))))
	mux.Handle("GET /admin/bin", browserAuthn(adminAuthz(endpoint.AdminBin(config.BrowserContext, config.RecycleBin, config.Views))))
	mux.Handle("GET /admin/tags", browserAuthn(adminAuthz(endpoint.AdminTags(config.BrowserContext, config.Administration, config.Views))))
	mux.Handle("GET /admin/tokens", browserAuthn(adminAuthz(endpoint.AdminTokens(config.BrowserContext, config.Users, config.Tokens, config.Views))))
	mux.Handle("GET /admin/exports", browserAuthn(adminAuthz(endpoint.AdminExports(config.BrowserContext, config.Navigation, config.Views))))
	mux.Handle("GET /admin/images", browserAuthn(adminAuthz(endpoint.AdminImages(config.BrowserContext, config.Media, config.Views))))
	mux.Handle("GET /admin/attachments", browserAuthn(adminAuthz(endpoint.AdminAttachments(config.BrowserContext, config.Media, config.Views))))
	mux.Handle("POST /admin/attachments/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminAttachment(config.Media, config.Views))))
	mux.Handle("POST /admin/settings", browserAuthn(adminAuthz(endpoint.SaveAdminSettings(config.Settings, config.Views, config.Logger))))
	mux.Handle("POST /admin/editor-toolbar", browserAuthn(adminAuthz(endpoint.SaveAdminEditorToolbar(config.Settings, pluginManager, config.Views))))
	mux.Handle(
		"POST /admin/branding/logo",
		browserAuthn(adminAuthz(endpoint.SaveAdminBrandLogo(config.Settings, config.Logger))),
	)
	mux.Handle(
		"POST /admin/branding/logo/reset",
		browserAuthn(adminAuthz(endpoint.ResetAdminBrandLogo(config.Settings, config.Logger))),
	)
	mux.Handle("POST /admin/pdf", browserAuthn(adminAuthz(endpoint.SaveAdminPDFSettings(config.Settings, config.Logger))))
	mux.Handle("POST /admin/pdf/test", browserAuthn(adminAuthz(endpoint.TestAdminPDFService(config.Settings, config.Logger))))
	mux.Handle("POST /admin/pdf/headers/{id}/reveal", browserAuthn(adminAuthz(endpoint.RevealAdminPDFHeader(config.Settings, config.Logger))))
	mux.Handle(
		"POST /admin/authentication",
		browserAuthn(adminAuthz(endpoint.SaveAdminAuthentication(config.Settings, config.BrowserAuth, config.Views))),
	)

	mux.Handle("POST /admin/users/{id}", browserAuthn(adminAuthz(endpoint.UpdateAdminUser(config.Users, config.Views, config.Logger))))
	mux.Handle("POST /admin/users/{id}/sessions/revoke", browserAuthn(adminAuthz(endpoint.RevokeAdminUserSessions(config.Users, config.Logger))))
	mux.Handle("POST /admin/users/{id}/oidc/remove", browserAuthn(adminAuthz(endpoint.RemoveAdminOIDCIdentity(config.Users, config.Logger))))
	mux.Handle("POST /admin/oidc/pending/{id}/approve", browserAuthn(adminAuthz(endpoint.ApprovePendingOIDCIdentity(config.Users, config.Logger))))
	mux.Handle("POST /admin/oidc/pending/{id}/link", browserAuthn(adminAuthz(endpoint.LinkPendingOIDCIdentity(config.Users, config.Logger))))
	mux.Handle("POST /admin/oidc/pending/{id}/reject", browserAuthn(adminAuthz(endpoint.RejectPendingOIDCIdentity(config.Users, config.Logger))))
	mux.Handle("POST /admin/oidc/pending/{id}/reopen", browserAuthn(adminAuthz(endpoint.ReopenPendingOIDCIdentity(config.Users, config.Logger))))
	mux.Handle("POST /admin/groups", browserAuthn(adminAuthz(endpoint.CreateAdminGroup(config.Groups, config.Logger))))
	mux.Handle("POST /admin/groups/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminGroup(config.Groups, config.Logger))))
	mux.Handle("POST /admin/navigation", browserAuthn(adminAuthz(endpoint.SaveAdminNavigationIcon(config.Navigation, config.Logger))))
	mux.Handle("POST /admin/bin/restore/{slug...}", browserAuthn(adminAuthz(endpoint.RestoreAdminPage(config.RecycleBin, config.Logger))))
	mux.Handle("POST /admin/bin/delete/{slug...}", browserAuthn(adminAuthz(endpoint.PermanentlyDeleteAdminPage(config.RecycleBin, config.Logger))))
	mux.Handle("POST /admin/tags/{id}/delete", browserAuthn(adminAuthz(endpoint.DeleteAdminTag(config.Administration, config.Logger))))
	mux.Handle("POST /admin/tokens", browserAuthn(adminAuthz(endpoint.CreateAdminToken(config.Tokens, config.Logger))))
	mux.Handle("DELETE /admin/tokens/{id}", browserAuthn(adminAuthz(endpoint.DeleteAdminToken(config.Tokens, config.Logger))))
	mux.Handle(
		"POST /admin/export",
		browserAuthn(adminAuthz(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger))),
	)

	mux.Handle("GET /api/admin/users", apiAuthn(adminAuthz(endpoint.SearchAdminUsers(config.Users, config.Logger))))
	mux.Handle(
		"GET /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AdminGroupMembers(config.Groups, config.Logger))),
	)
	mux.Handle(
		"POST /api/admin/groups/{id}/members",
		apiAuthn(adminAuthz(endpoint.AddAdminGroupMember(config.Groups, config.Users, config.Logger))),
	)
	mux.Handle(
		"DELETE /api/admin/groups/{id}/members/{userID}",
		apiAuthn(adminAuthz(endpoint.RemoveAdminGroupMember(config.Groups, config.Logger))),
	)
	mux.Handle(
		"POST /api/admin/export",
		apiAuthn(adminAuthz(endpoint.ExportPortablePages(config.PageLookup, config.Navigation, config.Media, config.Logger))),
	)
	mux.Handle(
		"DELETE /api/admin/bin/{slug...}",
		apiAuthn(adminAuthz(endpoint.PermanentlyDeletePage(config.RecycleBin, config.Logger))),
	)
}
