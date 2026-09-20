package handler

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
	"github.com/kumbuka-me/sdk"
)

// previewRequest contains Markdown submitted for server-side editor preview.
type previewRequest struct {
	// Markdown is the unsaved Markdown source to render.
	Markdown string `json:"markdown"`
	// Slug is the current unsaved page path used by dynamic page functions.
	Slug string `json:"slug"`
}

// pageRequest contains the mutable page fields accepted by the JSON API.
type pageRequest struct {
	// Slug supplies the desired page path for create requests.
	Slug string `json:"slug"`
	// ExpectedUpdatedAt is the updated_at value returned when an existing page was read.
	ExpectedUpdatedAt time.Time `json:"expected_updated_at"`
	// Title is the required page title.
	Title string `json:"title"`
	// Icon is the optional icon displayed with the page title.
	Icon string `json:"icon"`
	// Language optionally overrides the default content language.
	Language string `json:"language"`
	// Markdown is the page Markdown body.
	Markdown string `json:"markdown_content"`
	// Tags contains the requested page tags.
	Tags []string `json:"tags"`
	// GroupIDs contains collaboration groups assigned to the page.
	GroupIDs []int64 `json:"group_ids"`
	// Message describes the revision being created.
	Message string `json:"message"`
	// Status is the page lifecycle state.
	Status string `json:"status"`
	// OwnerGroupID optionally assigns documentation ownership to a group.
	OwnerGroupID int64 `json:"owner_group_id"`
	// ReviewIntervalDays configures documentation review cadence.
	ReviewIntervalDays int `json:"review_interval_days"`
	// MarkReviewed records a review at save time.
	MarkReviewed bool `json:"mark_reviewed"`
	// DeprecatedTarget points deprecated content at its replacement.
	DeprecatedTarget string `json:"deprecated_target"`
	// Properties contains structured page metadata.
	Properties map[string]string `json:"properties"`
}

// PreviewMarkdown renders unsaved Markdown with the same resolver used by persisted pages.
func PreviewMarkdown(
	navigationUseCases navigationService,
	catalogUseCases pageReportCatalogService,
	accessUseCases pageAccessReader,
	renderer *md.Renderer,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		request, err := decode[previewRequest](w, r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid JSON request.", "request", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		options := md.DefaultOptions()

		slug := md.Slug(request.Slug)
		user := currentUser(r)
		securedCatalog := accessiblePageCatalog{catalog: catalogUseCases, access: accessUseCases, user: user}
		pageNavigation, err := subpageNavigation(r.Context(), navigationUseCases, accessUseCases, user, slug)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		rendered, err := renderer.RenderPageResolvedWithFunctions(
			request.Markdown,
			md.Slug,
			options,
			md.Functions{
				Context:      r.Context(),
				Capabilities: plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog()),
			},
		)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, map[string]string{"html": rendered.HTML})
	}
}

// subpageNavigation prepares the permission-filtered navigation data for plugin capabilities.
func subpageNavigation(
	ctx context.Context,
	navigationUseCases navigationService,
	accessUseCases pageAccessReader,
	user domain.User,
	slug string,
) ([]sdk.NavigationNode, error) {
	pages, err := navigationUseCases.NavigationPages(ctx)
	if err != nil {
		return nil, err
	}
	pages, err = accessUseCases.FilterPages(ctx, user, pages)
	if err != nil {
		return nil, err
	}

	icons, err := navigationUseCases.NavigationIcons(ctx)
	if err != nil {
		return nil, err
	}

	items := make([]navigation.Page, 0, len(pages))
	for _, page := range pages {
		items = append(items, navigation.Page{Slug: page.Slug, Title: page.Title, Icon: page.Icon})
	}

	tree := navigation.Build(items, navigation.Options{Icons: icons})
	return plugincap.Navigation(navigation.Children(tree, slug), pageURL), nil
}

// pageURL returns the server route for one page slug.
func pageURL(slug string) string {
	return "/pages/" + strings.Trim(slug, "/")
}

// ListPages returns recently updated pages up to the requested limit.
func ListPages(catalogUseCases pageListService, accessUseCases pageAccessReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		pages, err := catalogUseCases.ListPages(r.Context(), 100)
		if err == nil {
			pages, err = accessUseCases.FilterPages(r.Context(), currentUser(r), pages)
		}
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		stripMarkdown(pages)
		httpresponse.Respond(w, http.StatusOK, jsonSlice(pages))
	}
}

// GetPage returns a page by slug.
func GetPage(catalogUseCases pageLookupService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if rawSlug, ok := strings.CutSuffix(slug, "/raw"); ok {
			page, err := catalogUseCases.GetPage(r.Context(), rawSlug)
			if err != nil {
				writePageProblem(logger, w, err)
				return
			}

			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")

			_, _ = w.Write([]byte(page.Markdown))

			return
		}

		page, err := catalogUseCases.GetPage(r.Context(), slug)

		if errors.Is(err, domain.ErrNotFound) {
			if target, aliasErr := catalogUseCases.ResolvePageAlias(r.Context(), slug); aliasErr == nil {
				w.Header().Set("Content-Location", "/api/pages/"+target)
				page, err = catalogUseCases.GetPage(r.Context(), target)
			} else if !errors.Is(aliasErr, domain.ErrNotFound) {
				httpresponse.InternalServerError(logger, w, aliasErr)
				return
			}
		}

		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, page)
	}
}

// SavePage persists page content, revision history, tags, and links transactionally.
func SavePage(pageUseCases pageWriterService, accessUseCases pageAccessReader, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		request, err := decode[pageRequest](w, r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid JSON request.", "request", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		if r.Method == http.MethodPut && request.ExpectedUpdatedAt.IsZero() {
			httpresponse.Problem(
				w,
				http.StatusBadRequest,
				"Page validation failed.",
				httpresponse.NewFieldProblem("expected_updated_at", "Supply the updated_at value returned by GET /api/pages/{slug} before updating a page."),
			)
			return
		}

		slug := cmp.Or(r.PathValue("slug"), request.Slug)
		allowed, accessErr := accessUseCases.CanEdit(r.Context(), user, slug)
		if accessErr != nil {
			httpresponse.InternalServerError(logger, w, accessErr)
			return
		}
		if !allowed {
			httpresponse.Problem(w, http.StatusForbidden, "You do not have permission to edit this page path.")
			return
		}

		page, err := pageUseCases.Save(r.Context(), apppages.PageSaveInput{
			PreviousSlug:       r.PathValue("slug"),
			ExpectedUpdatedAt:  request.ExpectedUpdatedAt,
			Slug:               slug,
			Title:              request.Title,
			Icon:               request.Icon,
			Language:           request.Language,
			Markdown:           request.Markdown,
			Message:            request.Message,
			Tags:               request.Tags,
			GroupIDs:           request.GroupIDs,
			Status:             request.Status,
			OwnerGroupID:       request.OwnerGroupID,
			ReviewIntervalDays: request.ReviewIntervalDays,
			MarkReviewed:       request.MarkReviewed,
			DeprecatedTarget:   request.DeprecatedTarget,
			Properties:         request.Properties,
			Actor:              user,
		})
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		status := http.StatusOK

		if r.Method == http.MethodPost {
			status = http.StatusCreated
			w.Header().Set("Location", "/api/pages/"+page.Slug)
		}

		httpresponse.Respond(w, status, page)
	}
}

// DeletePage removes a page by slug.
func DeletePage(pageUseCases pageWriterService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		if err := pageUseCases.Delete(r.Context(), r.PathValue("slug"), user); err != nil {
			writePageProblem(logger, w, err)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// PermanentlyDeletePage removes a page already held in the recycle bin.
func PermanentlyDeletePage(recycleBinUseCases recycleBinService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := recycleBinUseCases.PermanentlyDeletePage(r.Context(), r.PathValue("slug")); err != nil {
			writePageProblem(logger, w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// stripMarkdown removes Markdown bodies from page summaries.
func stripMarkdown(pages []domain.Page) {
	for index := range pages {
		pages[index].Markdown = ""
	}
}
