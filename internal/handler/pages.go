package handler

import (
	"cmp"
	"context"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/domain"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	md "github.com/kumbuka-me/kumbuka/internal/markdown"
	"github.com/kumbuka-me/kumbuka/internal/navigation"
	"github.com/kumbuka-me/kumbuka/internal/plugincap"
	"github.com/kumbuka-me/kumbuka/internal/service"
)

// Home renders the dashboard for the current user.
func Home(
	viewDataUseCases viewDataService,
	catalogUseCases homeCatalogService,
	draftUseCases draftListService,
	accessUseCases pageAccessReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.User(r)

		favorites, err := catalogUseCases.Favorites(r.Context(), user.ID)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		recent, err := catalogUseCases.ListPages(r.Context(), 8)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		viewed, err := catalogUseCases.RecentViewed(r.Context(), user.ID, 8)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		popular, err := catalogUseCases.Popular(r.Context(), 8)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		recentEdits, err := catalogUseCases.RecentEdited(r.Context(), user.ID, 6)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		for _, collection := range []*[]domain.Page{&favorites, &recent, &viewed, &popular} {
			filtered, filterErr := accessUseCases.FilterPages(r.Context(), user, *collection)
			if filterErr != nil {
				httpresponse.InternalServerError(views.logger, w, filterErr)
				return
			}
			*collection = filtered
		}
		recentEdits, err = visibleRecentEdits(r.Context(), accessUseCases, user, recentEdits)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		var drafts []domain.PageDraft

		if user.Role == "admin" || user.Role == "editor" {
			drafts, err = draftUseCases.List(r.Context(), user.ID, 6)
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}
		}

		data, err := viewData(r, viewDataUseCases, views, "Home")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.Favorites, data.Recent, data.Pages, data.Popular = favorites, recent, viewed, popular
		data.RecentEdits = recentEdits
		data.Drafts = drafts

		render(views, w, "home", data)
	}
}

// ViewPage renders one readable page and its active plugin detail widgets.
func ViewPage(
	viewDataUseCases viewDataService,
	catalogUseCases pageViewCatalogService,
	accessUseCases pageAccessReader,
	approvalUseCases pageApprovalService,
	renderer *md.Renderer,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		r, timingTrace := views.startPageTiming(r)
		defer views.logPageTiming(timingTrace, r, slug)

		stop := measurePageStage(r.Context(), "page_lookup")
		page, alias, err := getPageOrAlias(r.Context(), catalogUseCases, slug)
		stop()
		if errors.Is(err, domain.ErrNotFound) {
			renderNotFoundPage(w, r, viewDataUseCases, views)
			return
		}
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		if alias != "" {
			http.Redirect(w, r, "/pages/"+alias, http.StatusPermanentRedirect)
			return
		}

		user, _ := auth.User(r)
		securedCatalog := accessiblePageCatalog{catalog: catalogUseCases, access: accessUseCases, user: user}

		stop = measurePageStage(r.Context(), "record_view")
		_ = catalogUseCases.RecordView(r.Context(), slug, user.ID)
		stop()

		stop = measurePageStage(r.Context(), "favorite_lookup")
		pageFavorite, err := catalogUseCases.IsFavorite(r.Context(), slug, user.ID)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "page_watch")
		pageWatch, err := catalogUseCases.PageWatch(r.Context(), slug, user.ID)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "review_request")
		reviewRequest, err := approvalUseCases.PageReviewRequest(r.Context(), slug)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}
		stop = measurePageStage(r.Context(), "can_review")
		canReview, err := approvalUseCases.CanReview(r.Context(), slug, user)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}
		stop = measurePageStage(r.Context(), "can_edit")
		canEditPage, err := accessUseCases.CanEdit(r.Context(), user, slug)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		canManageReview := canEditPage && approvalUseCases.CanManageReview(reviewRequest, user)

		var reviewGroups []domain.Group
		if canEditPage && (reviewRequest.ID == 0 || canManageReview) {
			stop = measurePageStage(r.Context(), "review_groups")
			reviewGroups, err = approvalUseCases.ReviewGroups(r.Context())
			stop()
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}
		}

		options := md.DefaultOptions()

		stop = measurePageStage(r.Context(), "outgoing_links")
		outgoingLinks, err := catalogUseCases.PageLinks(r.Context(), slug)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "view_data")
		data, err := viewData(r, viewDataUseCases, views, page.Title)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		var comments []domain.PageComment

		if data.ApplicationSettings.DiscussionsEnabled {
			stop = measurePageStage(r.Context(), "comments")
			comments, err = catalogUseCases.PageComments(r.Context(), slug)
			stop()
			if err != nil {
				writePageProblem(views.logger, w, err)
				return
			}
		}

		data.PageContentLanguage = cmp.Or(page.Language, data.PageContentLanguage)

		stop = measurePageStage(r.Context(), "page_navigation")
		pageNavigation := plugincap.Navigation(
			navigation.Children(data.Navigation, slug),
			pageURL,
		)
		capabilities := plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog())
		stop()

		fingerprint := renderer.RenderFingerprint(options)
		persistable := renderer.CanPersist(page.Markdown, page.PluginUsage)
		var rendered md.RenderedPage

		if persistable && page.Render.Fingerprint == fingerprint {
			stop = measurePageStage(r.Context(), "render_artifact_hit")
			rendered = renderedPageFromArtifact(page.Render)
			stop()
		} else {
			stop = measurePageStage(r.Context(), "markdown")
			rendered, err = renderer.RenderPageResolvedWithFunctions(
				page.Markdown,
				md.Slug,
				options,
				md.Functions{
					Context:      r.Context(),
					PluginUsage:  page.PluginUsage,
					Capabilities: capabilities,
				},
			)
			stop()
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}
			if persistable {
				if artifact, ok := pageRenderArtifact(rendered, fingerprint); ok {
					stop = measurePageStage(r.Context(), "render_artifact_store")
					artifactErr := catalogUseCases.SavePageRender(r.Context(), page.ID, page.UpdatedAt, artifact)
					stop()
					if artifactErr != nil {
						views.logger.Warn("store page render artifact", "event", "page_render_store_failed", "slug", page.Slug, "error", artifactErr)
					}
				}
			}
		}

		stop = measurePageStage(r.Context(), "broken_links")
		renderedHTML := rendered.HTML
		for _, link := range outgoingLinks {
			if link.Exists {
				continue
			}

			renderedHTML = strings.ReplaceAll(
				renderedHTML,
				`<a href="/pages/`+link.TargetSlug+`"`,
				`<a class="wiki-link-broken" href="/pages/`+link.TargetSlug+`"`,
			)
		}
		stop()

		data.Page, data.HTML = &page, template.HTML(renderedHTML)
		data.PageReviewRequest = reviewRequest
		data.CanReviewPage = canReview
		data.CanManageReview = canManageReview
		data.ReviewGroups = reviewGroups
		data.CanEdit = canEditPage
		data.PluginInspectors = rendered.Inspectors
		data.PluginExportFields = rendered.ExportFields
		data.Comments = comments
		data.PageFavorite = pageFavorite
		data.PageWatchScope = pageWatch.Scope
		data.PageContents = rendered.Contents

		stop = measurePageStage(r.Context(), "page_detail_widgets")
		pageValue := plugincap.PageValue(page)
		widgets, err := renderer.RenderWidgets(r.Context(), "page.details", &pageValue, data.PluginFeatures, capabilities)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.PageDetailWidgets = widgetViews(widgets)

		stop = measurePageStage(r.Context(), "template_render")
		render(views, w, "page", data)
		stop()
	}
}

// getPageOrAlias resolves a page directly or returns the target of a matching alias.
func getPageOrAlias(
	ctx context.Context,
	catalogUseCases pageViewCatalogService,
	slug string,
) (domain.Page, string, error) {
	page, err := catalogUseCases.GetPage(ctx, slug)
	if err == nil {
		return page, "", nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return domain.Page{}, "", err
	}

	target, err := catalogUseCases.ResolvePageAlias(ctx, slug)
	if err != nil {
		return domain.Page{}, "", err
	}

	return domain.Page{}, target, nil
}

// EditPage renders the page creation or editing form.
func EditPage(
	viewDataUseCases viewDataService,
	catalogUseCases pageContentService,
	groupUseCases groupReader,
	templateUseCases templateService,
	accessUseCases pageAccessReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)

		data, err := viewData(r, viewDataUseCases, views, "New page")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		groups, err := groupUseCases.AssignableGroups(r.Context(), user)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.Groups = groups
		data.ContentLanguages = contentLanguageOptions
		data.PageStatuses = domain.PageStatuses()

		switch slug := r.PathValue("slug"); slug {
		case "":
			if err := prepareNewPageEditor(r, &data, templateUseCases); err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}

		default:
			page, err := catalogUseCases.GetPage(r.Context(), slug)
			if errors.Is(err, domain.ErrNotFound) {
				renderNotFoundPage(w, r, viewDataUseCases, views)
				return
			}
			if err != nil {
				writePageProblem(views.logger, w, err)
				return
			}

			data.Title = "Edit " + page.Title
			data.Page = &page
			data.EditorInitialSlug = page.Slug
			data.EditorParentPath, data.EditorPathSegment = splitPagePath(page.Slug)
			data.PagePathOptions = pagePathOptions(data.Navigation, page.Slug)
			data.PageContentLanguage = cmp.Or(page.Language, data.PageContentLanguage)
		}

		render(views, w, "edit", data)
	}
}

// prepareNewPageEditor initializes editor state used only when creating a page.
func prepareNewPageEditor(
	r *http.Request,
	data *ViewData,
	templateUseCases templateService,
) error {
	data.PagePathOptions = pagePathOptions(data.Navigation, "")

	prefillSlug := md.Slug(r.URL.Query().Get("slug"))

	switch prefillSlug {
	case "":
		parent := md.Slug(r.URL.Query().Get("parent"))
		if hasPagePathOption(data.PagePathOptions, parent) {
			data.EditorParentPath = parent
		}

	default:
		data.EditorInitialSlug = prefillSlug
		data.EditorParentPath, data.EditorPathSegment = splitPagePath(prefillSlug)
		data.PagePathOptions = ensurePagePathOption(
			data.PagePathOptions,
			data.EditorParentPath,
		)
	}

	templates, err := templateUseCases.PageTemplates(r.Context())
	if err != nil {
		return err
	}

	data.PageTemplates = templates

	value := r.URL.Query().Get("template")
	if value == "" {
		return nil
	}

	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return nil
	}

	selected, err := templateUseCases.PageTemplate(r.Context(), id)

	switch {
	case err == nil:
		data.EditorTemplate = &selected

	case errors.Is(err, domain.ErrNotFound):
		return nil

	default:
		return err
	}

	return nil
}

// ensurePagePathOption adds a path option when it does not already exist.
func ensurePagePathOption(options []pagePathOption, slug string) []pagePathOption {
	if slug == "" || hasPagePathOption(options, slug) {
		return options
	}

	return append(options, pagePathOption{
		Slug:  slug,
		Label: strings.ReplaceAll(slug, "/", " / "),
	})
}

// splitPagePath separates a page slug into its parent path and final segment.
func splitPagePath(slug string) (string, string) {
	slug = strings.Trim(strings.TrimSpace(slug), "/")

	if parent, segment, ok := strings.CutLast(slug, "/"); ok {
		return parent, segment
	}

	return "", slug
}

// SavePageForm creates or updates a page from the browser form.
func SavePageForm(
	pageUseCases pageWriterService,
	draftUseCases draftDiscardService,
	templateUseCases templateService,
	accessUseCases pageAccessReader,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)

		r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid form.")
			return
		}

		originalSlug := strings.TrimSpace(r.FormValue("original_slug"))
		destinationSlug := md.Slug(r.FormValue("slug"))
		for _, path := range []string{originalSlug, destinationSlug} {
			if path == "" {
				continue
			}
			allowed, accessErr := accessUseCases.CanEdit(r.Context(), user, path)
			if accessErr != nil {
				httpresponse.InternalServerError(views.logger, w, accessErr)
				return
			}
			if !allowed {
				httpresponse.Problem(w, http.StatusForbidden, "You do not have permission to edit this page path.")
				return
			}
		}

		metadata, err := pageMetadataFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(
				w,
				http.StatusBadRequest,
				"Page validation failed.",
				"",
				err,
			) {
				return
			}

			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		markdown := r.FormValue("markdown")
		if originalSlug == "" {
			markdown, err = resolvePageTemplateFields(r.Context(), r, templateUseCases, markdown)
			if err != nil {
				writePageProblem(views.logger, w, err)
				return
			}
		}

		properties := pagePropertiesFromForm(r)

		page, err := pageUseCases.Save(r.Context(), service.PageSaveInput{
			PreviousSlug:       originalSlug,
			Slug:               r.FormValue("slug"),
			Title:              r.FormValue("title"),
			Icon:               r.FormValue("icon"),
			Language:           r.FormValue("language"),
			Markdown:           markdown,
			Message:            r.FormValue("message"),
			Tags:               splitTags(r.FormValue("tags")),
			GroupIDs:           parseGroupIDs(r.Form["group_id"]),
			Status:             metadata.Status,
			OwnerGroupID:       metadata.OwnerGroupID,
			ReviewIntervalDays: metadata.ReviewIntervalDays,
			MarkReviewed:       metadata.MarkReviewed,
			DeprecatedTarget:   metadata.DeprecatedTarget,
			Properties:         properties,
			Actor:              user,
		})
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		draftKey := "new"

		if originalSlug != "" {
			draftKey = service.PageDraftKey(page.ID)
		}

		if err := draftUseCases.Delete(r.Context(), user.ID, draftKey); err != nil {
			views.logger.Warn(
				"discard saved page draft",
				"event", "page_draft_cleanup_failed",
				"draft_key", draftKey,
				"user_id", user.ID,
				"error", err,
			)
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}

// resolvePageTemplateFields validates and materializes creation-time blueprint fields.
func resolvePageTemplateFields(
	ctx context.Context,
	r *http.Request,
	templates templateService,
	markdown string,
) (string, error) {
	value := strings.TrimSpace(r.FormValue("template_id"))
	if value == "" {
		return markdown, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return "", &service.ValidationError{Fields: []service.FieldError{{Field: "template", Message: "Choose a valid page template."}}}
	}
	template, err := templates.PageTemplate(ctx, id)
	if err != nil {
		return "", err
	}
	validation := &service.ValidationError{}
	for _, field := range template.Fields {
		fieldValue := r.FormValue("blueprint_" + field.Name)
		if field.Required && strings.TrimSpace(fieldValue) == "" {
			validation.Fields = append(validation.Fields, service.FieldError{Field: "blueprint_" + field.Name, Message: field.Label + " is required."})
		}
		markdown = strings.ReplaceAll(markdown, "{{field:"+field.Name+"}}", fieldValue)
	}
	if len(validation.Fields) > 0 {
		return "", validation
	}
	return markdown, nil
}

// DeletePageForm deletes a page from the browser and returns home.
func DeletePageForm(
	pageUseCases pageWriterService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		slug := r.PathValue("slug")

		if err := pageUseCases.Delete(r.Context(), slug, user); err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// FavoritePage updates the current user's favorite status for a page.
func FavoritePage(
	catalogUseCases favoriteService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := r.PathValue("slug")

		slug, ok := strings.CutSuffix(value, "/favorite")
		if !ok {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		user, _ := auth.User(r)

		if err := catalogUseCases.SetFavorite(
			r.Context(),
			slug,
			user.ID,
			r.FormValue("on") != "false",
		); err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// WatchPage updates the current user's page or subtree subscription.
func WatchPage(
	catalogUseCases pageWatchService,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.User(r)
		slug := strings.Trim(strings.TrimSpace(r.PathValue("slug")), "/")
		scope := strings.TrimSpace(r.FormValue("scope"))

		if scope != "" && scope != domain.PageWatchScopePage && scope != domain.PageWatchScopeSubtree {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"Watch settings are invalid.",
				httpresponse.NewFieldProblem("scope", "Choose page or subtree notifications."),
			)
			return
		}

		if err := catalogUseCases.SetPageWatch(r.Context(), slug, user.ID, scope); err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// splitTags normalizes a comma-separated tag list.
func splitTags(value string) []string {
	result := make([]string, 0)

	for part := range strings.SplitSeq(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}

	return result
}

// parseGroupIDs parses positive group identifiers from form values and leaves invalid values for store validation.
func parseGroupIDs(values []string) []int64 {
	groupIDs := make([]int64, 0, len(values))

	for _, value := range values {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			groupIDs = append(groupIDs, -1)
			continue
		}

		groupIDs = append(groupIDs, id)
	}

	return groupIDs
}

// pageMetadataFromForm parses page lifecycle and review metadata.
func pageMetadataFromForm(r *http.Request) (domain.PageMetadata, error) {
	status := strings.TrimSpace(r.FormValue("status"))
	if !domain.ValidPageStatus(status) {
		return domain.PageMetadata{}, newRequestError(
			"status",
			"Choose a valid page status.",
			errors.New("invalid page status"),
		)
	}

	var ownerGroupID int64

	if value := strings.TrimSpace(r.FormValue("owner_group_id")); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			return domain.PageMetadata{}, newRequestError(
				"owner_group_id",
				"Choose a valid owner group.",
				errors.New("invalid owner group"),
			)
		}

		ownerGroupID = parsed
	}

	interval := 0

	if value := strings.TrimSpace(r.FormValue("review_interval_days")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return domain.PageMetadata{}, newRequestError(
				"review_interval_days",
				"Choose a valid review interval.",
				errors.New("invalid review interval"),
			)
		}

		if !domain.ValidReviewIntervalDays(parsed) {
			return domain.PageMetadata{}, newRequestError(
				"review_interval_days",
				"Choose a valid review interval.",
				errors.New("invalid review interval"),
			)
		}

		interval = parsed
	}

	return domain.PageMetadata{
		Status:             status,
		OwnerGroupID:       ownerGroupID,
		ReviewIntervalDays: interval,
		MarkReviewed:       r.FormValue("mark_reviewed") == "on",
		DeprecatedTarget:   md.Slug(r.FormValue("deprecated_target")),
	}, nil
}

// pagePropertiesFromForm returns normalized structured page properties.
func pagePropertiesFromForm(r *http.Request) map[string]string {
	keys := r.Form["property_key"]
	values := r.Form["property_value"]
	properties := map[string]string{}

	for index, key := range keys {
		key = strings.TrimSpace(key)

		if key == "" || index >= len(values) {
			continue
		}

		value := strings.TrimSpace(values[index])

		if value != "" {
			properties[key] = value
		}
	}

	return properties
}

// writePageProblem translates page-domain errors into HTTP problems.
func writePageProblem(
	logger *slog.Logger,
	w http.ResponseWriter,
	err error,
) {
	if assignment, ok := errors.AsType[*domain.GroupAssignmentError](err); ok {
		httpresponse.Problem(w,
			http.StatusForbidden,
			"The selected page groups are not assignable.",
			httpresponse.NewFieldProblem(
				assignment.Field,
				"Choose groups you are allowed to assign.",
			),
		)
		return
	}

	if tryWriteValidationProblem(w, err, "Page validation failed.") {
		return
	}

	switch {
	case errors.Is(err, domain.ErrRevisionNotFound):
		httpresponse.Problem(w,
			http.StatusNotFound,
			"Revision not found.",
		)

	case errors.Is(err, domain.ErrCommentNotFound):
		httpresponse.Problem(w,
			http.StatusNotFound,
			"Comment not found.",
		)

	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w,
			http.StatusNotFound,
			"Page not found.",
		)

	case errors.Is(err, domain.ErrAlreadyExists):
		httpresponse.Problem(w,
			http.StatusConflict,
			"Page path already exists.",
			httpresponse.NewFieldProblem(
				"slug",
				"Choose a different page path.",
			),
		)

	case errors.Is(err, domain.ErrForbidden):
		httpresponse.Problem(w,
			http.StatusForbidden,
			"The page operation is not permitted.",
		)

	case errors.Is(err, domain.ErrPageInBin):
		httpresponse.Problem(w,
			http.StatusConflict,
			"This page path is currently in the recycle bin.",
			httpresponse.NewFieldProblem(
				"slug",
				"Restore the deleted page or choose a different path.",
			),
		)

	case errors.Is(err, service.ErrDiscussionsDisabled):
		httpresponse.Problem(w,
			http.StatusForbidden,
			"Page discussions are disabled.",
		)

	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
