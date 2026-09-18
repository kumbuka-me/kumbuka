package handler

import (
	"cmp"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// createPageShareLinkResponse contains the response payload for create page share link response.
type createPageShareLinkResponse struct {
	// URL is the target URL for create page share link response.
	URL string `json:"url"`
}

// CreatePageShareLink creates a reusable public permalink for the selected page.
func CreatePageShareLink(sharingUseCases sharingService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issued, err := sharingUseCases.CreatePageShareLink(
			r.Context(),
			r.PathValue("slug"),
			currentUser(r),
		)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusCreated, createPageShareLinkResponse{
			URL: "/share/" + issued.Token,
		})
	}
}

// SharedPage serves a reusable public permalink without authentication.
func SharedPage(
	sharingUseCases sharingService,
	catalogUseCases pageReportCatalogService,
	settingsUseCases settingsService,
	mediaUseCases imageContentService,
	renderer *md.Renderer,
	views *webview.Views,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		publicShareHeaders(w)

		link, err := sharingUseCases.PageShareLink(r.Context(), strings.TrimSpace(r.PathValue("token")))
		if err != nil {
			writePublicShareError(logger, w, err)
			return
		}

		renderSharedPage(
			w,
			r,
			catalogUseCases,
			settingsUseCases,
			mediaUseCases,
			renderer,
			views,
			logger,
			link.Slug,
		)
	}
}

// renderSharedPage renders a standalone page without authenticated navigation or controls.
func renderSharedPage(
	w http.ResponseWriter,
	r *http.Request,
	catalogUseCases pageReportCatalogService,
	settingsUseCases settingsService,
	mediaUseCases imageContentService,
	renderer *md.Renderer,
	views *webview.Views,
	logger *slog.Logger,
	slug string,
) {
	page, err := catalogUseCases.GetPage(r.Context(), slug)
	if err != nil {
		writePublicShareError(logger, w, err)
		return
	}

	application, err := settingsUseCases.ApplicationSettings(r.Context())
	if err != nil {
		writePublicShareError(logger, w, err)
		return
	}
	options := md.DefaultOptions()

	rendered, err := renderer.RenderPageResolvedWithFunctions(
		page.Markdown,
		md.Slug,
		options,
		md.Functions{
			Context:      r.Context(),
			PluginUsage:  page.PluginUsage,
			Capabilities: plugincap.Capabilities(plugincap.SharedPages{Source: catalogUseCases, Slug: slug}, nil, renderer.IconCatalog()),
		},
	)
	if err != nil {
		writePublicShareError(logger, w, err)
		return
	}

	standaloneHTML, err := inlineRenderedMedia(r.Context(), mediaUseCases, rendered.HTML)
	if err != nil {
		writePublicShareError(logger, w, err)
		return
	}

	data, err := views.PublicPluginData(page.Title, renderer.PluginManager())
	if err != nil {
		writePublicShareError(logger, w, err)
		return
	}

	data.Page = &page
	data.HTML = template.HTML(standaloneHTML)
	data.ApplicationSettings = application
	data.PageContentLanguage = cmp.Or(page.Language, application.ContentLanguage)

	views.RenderTemplate(w, "shared_page", "shared-layout", data)
}

// publicShareHeaders prevent public bearer URLs from leaking through caches, referrers, or indexing.
func publicShareHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow, noarchive")
}

// writePublicShareError hides whether an invalid, revoked, or deleted share link ever existed.
func writePublicShareError(logger *slog.Logger, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, "Share link not found or no longer available.")
	default:
		httpresponse.InternalServerError(logger.With("operation", "public_share"), w, err)
	}
}
