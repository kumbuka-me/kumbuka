package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// registerPageRoutes registers page editing, collaboration, review, and export workflows.
func registerPageRoutes(mux *http.ServeMux, config Config) {
	browserAuthn := middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator)
	adminAuthz := middleware.RequireRole(domain.UserRoleAdmin)
	editorAuthz := middleware.RequireRole(domain.UserRoleAdmin, domain.UserRoleEditor)
	browserAdmin := func(handler http.Handler) http.Handler { return browserAuthn(adminAuthz(handler)) }
	browserEditor := func(handler http.Handler) http.Handler { return browserAuthn(editorAuthz(handler)) }

	registerPluginPageRoutes(mux, config, browserAuthn)
	registerPageExportRoutes(mux, config, browserAuthn)
	registerPageCollaborationRoutes(mux, config, browserAuthn, browserEditor)
	registerPageEditingRoutes(mux, config, browserAuthn, browserEditor, browserAdmin)
	registerPageViewRoutes(mux, config, browserAuthn)
}

// registerPluginPageRoutes registers plugin actions and deferred page-fragment rendering.
func registerPluginPageRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	mux.Handle(
		"POST /plugins/actions/{pluginID}/{moduleID}/{actionID}",
		protect(endpoint.PluginWidgetCommand(config.PageReports, config.PageMutations, config.Navigation, config.Renderer, config.Notifications)),
	)
	mux.Handle(
		"GET /api/plugin-fragments/{pluginID}/{moduleID}/{index}/{slug...}",
		protect(endpoint.PluginMacroFragment(config.PageReports, config.Navigation, config.Renderer, config.Logger)),
	)
}

// registerPageExportRoutes registers Markdown, plugin, PDF, and preview export endpoints.
func registerPageExportRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	mux.Handle("GET /export/markdown/{slug...}", protect(endpoint.ExportPageMarkdown(config.PageLookup, config.Media, config.Logger)))
	mux.Handle("POST /export/plugin/{pluginID}/{moduleID}/{slug...}", protect(endpoint.ExportPagePlugin(config.PageReports, config.Navigation, config.Renderer, config.Logger)))

	exportPDF := protect(endpoint.ExportPagePDF(
		config.PageReports,
		config.Settings,
		config.Navigation,
		config.Media,
		config.Renderer,
		config.Views,
		config.Logger,
	))
	mux.Handle("GET /export/pdf/{slug...}", exportPDF)
	mux.Handle("POST /export/pdf/{slug...}", exportPDF)
	mux.Handle(
		"POST /export/preview/{slug...}",
		protect(endpoint.PreviewPageExport(
			config.PageReports,
			config.Settings,
			config.Navigation,
			config.Media,
			config.Renderer,
			config.Logger,
		)),
	)
}

// registerPageCollaborationRoutes registers review and discussion workflows.
func registerPageCollaborationRoutes(
	mux *http.ServeMux,
	config Config,
	browserAuthn middleware.Middleware,
	browserEditor middleware.Middleware,
) {
	mux.Handle("POST /pages/review/{slug...}", browserEditor(endpoint.ReviewPageForm(config.PageMutations, config.PageLookup, config.Logger)))
	mux.Handle("POST /pages/approval/request/{slug...}", browserEditor(endpoint.RequestPageReview(config.PageReviews, config.PageLookup, config.Logger)))
	mux.Handle("POST /pages/approval/update/{id}/{slug...}", browserEditor(endpoint.UpdatePageReview(config.PageReviews, config.PageLookup, config.Logger)))
	mux.Handle("POST /pages/approval/cancel/{id}/{slug...}", browserEditor(endpoint.CancelPageReview(config.PageReviews, config.PageLookup, config.Logger)))
	mux.Handle("POST /pages/approval/decide/{id}/{slug...}", browserEditor(endpoint.DecidePageReview(config.PageReviews, config.PageLookup, config.Logger)))
	mux.Handle("GET /reviews/{id}/{slug...}", browserEditor(endpoint.PageReview(config.BrowserContext, config.PageReviewDiscussions, config.Views)))
	mux.Handle("POST /reviews/{id}/comments/{slug...}", browserEditor(endpoint.AddPageReviewComment(config.PageReviewDiscussions, config.Views)))
	mux.Handle("POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}", browserEditor(endpoint.ApplyPageReviewSuggestion(config.PageReviewDiscussions, config.Views)))
	mux.Handle("POST /reviews/{id}/suggestions/apply-all/{slug...}", browserEditor(endpoint.ApplyAllPageReviewSuggestions(config.PageReviewDiscussions, config.Views)))
	mux.Handle("POST /page-comments/{slug...}", browserAuthn(endpoint.AddPageComment(config.PageDiscussions, config.Views)))
	mux.Handle("POST /page-comments/suggestions/apply/{id}/{slug...}", browserEditor(endpoint.ApplyPageCommentSuggestion(config.PageDiscussions, config.Views)))
	mux.Handle("POST /page-comments/resolve/{id}/{slug...}", browserEditor(endpoint.ResolvePageComment(config.PageDiscussions, config.Views)))
}

// registerPageEditingRoutes registers page mutations, editor screens, favorites, watches, and revisions.
func registerPageEditingRoutes(
	mux *http.ServeMux,
	config Config,
	browserAuthn middleware.Middleware,
	browserEditor middleware.Middleware,
	browserAdmin middleware.Middleware,
) {
	mux.Handle("POST /pages/delete/{slug...}", browserAdmin(endpoint.DeletePageForm(config.PageMutations, config.Views)))
	mux.Handle("POST /pages/move/{slug...}", browserEditor(endpoint.MovePageForm(config.PageMutations, config.PageLookup, config.Logger)))

	editPage := endpoint.EditPage(config.BrowserContext, config.Editor, config.Views)
	mux.Handle("GET /pages/new", browserEditor(editPage))
	mux.Handle("GET /edit/{slug...}", browserEditor(editPage))
	mux.Handle("POST /pages", browserEditor(endpoint.SavePageForm(config.EditorSave, config.Views)))
	mux.Handle("POST /pages/{slug...}", browserAuthn(endpoint.FavoritePage(config.PagePersonal, config.Views)))
	mux.Handle("POST /page-watch/{slug...}", browserAuthn(endpoint.WatchPage(config.PagePersonal, config.Views)))
	mux.Handle("GET /revisions/{slug...}", browserAuthn(endpoint.RevisionHistory(config.PageHistory, config.Views)))
	mux.Handle("POST /revisions/{number}/restore/{slug...}", browserEditor(endpoint.RestoreRevision(config.PageMutations, config.Views)))
}

// registerPageViewRoutes registers canonical stable-ID pages and legacy slug redirects.
func registerPageViewRoutes(mux *http.ServeMux, config Config, protect middleware.Middleware) {
	viewPage := endpoint.ViewPage(
		config.BrowserContext,
		config.PageReports,
		config.PageRender,
		config.ViewPage,
		config.Renderer,
		config.Views,
	)
	canonicalPage := endpoint.CanonicalPage(config.PageLookup, config.Logger, viewPage)

	mux.Handle("GET /p/{id}", protect(canonicalPage))
	mux.Handle("GET /p/{id}/{slug...}", protect(canonicalPage))
	mux.Handle("GET /pages/{slug...}", protect(endpoint.LegacyPage(config.PageLookup, config.Logger)))
}
