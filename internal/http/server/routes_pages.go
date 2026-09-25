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

	mux.Handle(
		"POST /plugins/actions/{pluginID}/{moduleID}/{actionID}",
		browserAuthn(endpoint.PluginWidgetCommand(config.PageReports, config.Navigation, config.Renderer, config.Notifications)),
	)
	mux.Handle(
		"GET /export/markdown/{slug...}",
		browserAuthn(endpoint.ExportPageMarkdown(config.PageLookup, config.Media, config.Logger)),
	)
	mux.Handle(
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
	mux.Handle("GET /export/pdf/{slug...}", exportPDF)
	mux.Handle("POST /export/pdf/{slug...}", exportPDF)
	mux.Handle(
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

	mux.Handle("POST /pages/delete/{slug...}", browserAuthn(adminAuthz(endpoint.DeletePageForm(config.PageMutations, config.Views))))
	mux.Handle("POST /pages/move/{slug...}", browserAuthn(editorAuthz(endpoint.MovePageForm(config.PageMutations, config.Logger))))
	mux.Handle("POST /pages/review/{slug...}", browserAuthn(editorAuthz(endpoint.ReviewPageForm(config.PageMutations, config.Logger))))
	mux.Handle("POST /pages/approval/request/{slug...}", browserAuthn(editorAuthz(endpoint.RequestPageReview(config.PageReviews, config.Logger))))
	mux.Handle("POST /pages/approval/update/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.UpdatePageReview(config.PageReviews, config.Logger))))
	mux.Handle("POST /pages/approval/cancel/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.CancelPageReview(config.PageReviews, config.Logger))))
	mux.Handle("POST /pages/approval/decide/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.DecidePageReview(config.PageReviews, config.Logger))))
	mux.Handle("GET /reviews/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.PageReview(config.BrowserContext, config.PageReviewDiscussions, config.Views))))
	mux.Handle("POST /reviews/{id}/comments/{slug...}", browserAuthn(editorAuthz(endpoint.AddPageReviewComment(config.PageReviewDiscussions, config.Views))))
	mux.Handle("POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}", browserAuthn(editorAuthz(endpoint.ApplyPageReviewSuggestion(config.PageReviewDiscussions, config.Views))))
	mux.Handle("POST /reviews/{id}/suggestions/apply-all/{slug...}", browserAuthn(editorAuthz(endpoint.ApplyAllPageReviewSuggestions(config.PageReviewDiscussions, config.Views))))
	mux.Handle("POST /page-comments/{slug...}", browserAuthn(endpoint.AddPageComment(config.PageDiscussions, config.Views)))
	mux.Handle("POST /page-comments/suggestions/apply/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.ApplyPageCommentSuggestion(config.PageDiscussions, config.Views))))
	mux.Handle("POST /page-comments/resolve/{id}/{slug...}", browserAuthn(editorAuthz(endpoint.ResolvePageComment(config.PageDiscussions, config.Views))))

	editPage := endpoint.EditPage(config.BrowserContext, config.Editor, config.Views)
	mux.Handle("GET /pages/new", browserAuthn(editorAuthz(editPage)))
	mux.Handle("GET /edit/{slug...}", browserAuthn(editorAuthz(editPage)))
	mux.Handle(
		"POST /pages",
		browserAuthn(editorAuthz(endpoint.SavePageForm(config.EditorSave, config.Views))),
	)
	mux.Handle("POST /pages/{slug...}", browserAuthn(endpoint.FavoritePage(config.PagePersonal, config.Views)))
	mux.Handle("POST /page-watch/{slug...}", browserAuthn(endpoint.WatchPage(config.PagePersonal, config.Views)))
	mux.Handle("GET /revisions/{slug...}", browserAuthn(endpoint.RevisionHistory(config.PageHistory, config.Views)))
	mux.Handle(
		"POST /revisions/{number}/restore/{slug...}",
		browserAuthn(editorAuthz(endpoint.RestoreRevision(config.PageMutations, config.Views))),
	)
	mux.Handle(
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
