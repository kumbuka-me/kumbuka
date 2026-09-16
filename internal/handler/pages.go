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
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugincap"
)

// Home renders the dashboard for the current user.
func Home(
	viewDataUseCases viewDataService,
	catalogUseCases homeCatalogService,
	draftUseCases draftListService,
	accessUseCases pageAccessReader,
	renderer *md.Renderer,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := viewDataUseCases.Load(r, views, "Home")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		user, _ := auth.User(r)
		source := homeWidgetSource{catalog: catalogUseCases, drafts: draftUseCases, access: accessUseCases, user: user}
		capabilities := plugincap.MergeCapabilities(
			plugincap.Capabilities(nil, nil, renderer.IconCatalog()),
			plugincap.PageListCapabilities(source),
			plugincap.DraftCapabilities(source),
		)
		widgets, err := renderer.RenderWidgets(r.Context(), "home", nil, data.PluginFeatures, capabilities, data.Preferences.HiddenPluginWidgets)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		data.HomeWidgets = widgetViews(widgets)

		render(views, w, "home", data)
	}
}

// pageViewState contains user-specific page actions loaded before rendering a page.
type pageViewState struct {
	// favorite reports whether the current user has pinned the page.
	favorite bool
	// watch contains the current user's page-watch scope.
	watch domain.PageWatch
	// reviewRequest contains the active review workflow item, if any.
	reviewRequest domain.PageReviewRequest
	// canReview reports whether the current user may decide the active review.
	canReview bool
	// canEdit reports whether the current user may edit the page.
	canEdit bool
	// canManageReview reports whether the current user may update or cancel the active review.
	canManageReview bool
	// reviewGroups contains groups available as review targets.
	reviewGroups []domain.Group
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

		state, err := loadPageViewState(r.Context(), slug, user, catalogUseCases, accessUseCases, approvalUseCases)
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "outgoing_links")
		outgoingLinks, err := catalogUseCases.PageLinks(r.Context(), slug)
		stop()
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "view_data")
		data, err := viewDataUseCases.Load(r, views, page.Title)
		stop()
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		comments, err := loadPageComments(r.Context(), slug, data.ApplicationSettings.DiscussionsEnabled, catalogUseCases)
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}
		data.PageContentLanguage = cmp.Or(page.Language, data.PageContentLanguage)

		stop = measurePageStage(r.Context(), "page_navigation")
		pageNavigation := plugincap.Navigation(navigation.Children(data.Navigation, slug), pageURL)
		capabilities := plugincap.Capabilities(securedCatalog, pageNavigation, renderer.IconCatalog())
		stop()

		rendered, err := renderPageContent(r.Context(), page, md.DefaultOptions(), capabilities, renderer, catalogUseCases, views.logger)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		stop = measurePageStage(r.Context(), "broken_links")
		renderedHTML := markBrokenWikiLinks(rendered.HTML, outgoingLinks)
		stop()

		data.Page, data.HTML = &page, template.HTML(renderedHTML)
		data.PageReviewRequest = state.reviewRequest
		data.CanReviewPage = state.canReview
		data.CanManageReview = state.canManageReview
		data.ReviewGroups = state.reviewGroups
		data.CanEdit = state.canEdit
		data.PluginInspectors = rendered.Inspectors
		data.PluginExportFields = rendered.ExportFields
		data.Comments = comments
		data.PageFavorite = state.favorite
		data.PageWatchScope = state.watch.Scope
		data.PageContents = rendered.Contents
		if manager := renderer.PluginManager(); manager != nil {
			data.PluginPageActions = manager.PageActions(page.ID, page.Slug)
			data.PluginExporters = manager.Exporters(page.Slug)
		}

		stop = measurePageStage(r.Context(), "page_detail_widgets")
		pageValue := plugincap.PageValue(page)
		widgets, err := renderer.RenderWidgets(
			r.Context(),
			"page.details",
			&pageValue,
			data.PluginFeatures,
			capabilities,
			data.Preferences.HiddenPluginWidgets,
		)
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

// loadPageViewState loads user-specific favorite, watch, edit, and review state for a page.
func loadPageViewState(
	ctx context.Context,
	slug string,
	user domain.User,
	catalog pageViewCatalogService,
	access pageAccessReader,
	approvals pageApprovalService,
) (pageViewState, error) {
	var state pageViewState

	stop := measurePageStage(ctx, "favorite_lookup")
	favorite, err := catalog.IsFavorite(ctx, slug, user.ID)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.favorite = favorite

	stop = measurePageStage(ctx, "page_watch")
	watch, err := catalog.PageWatch(ctx, slug, user.ID)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.watch = watch

	stop = measurePageStage(ctx, "review_request")
	reviewRequest, err := approvals.PageReviewRequest(ctx, slug)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.reviewRequest = reviewRequest

	stop = measurePageStage(ctx, "can_review")
	canReview, err := approvals.CanReview(ctx, slug, user)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.canReview = canReview

	stop = measurePageStage(ctx, "can_edit")
	canEdit, err := access.CanEdit(ctx, user, slug)
	stop()
	if err != nil {
		return pageViewState{}, err
	}
	state.canEdit = canEdit
	state.canManageReview = canEdit && approvals.CanManageReview(reviewRequest, user)

	if canEdit && (reviewRequest.ID == 0 || state.canManageReview) {
		stop = measurePageStage(ctx, "review_groups")
		state.reviewGroups, err = approvals.ReviewGroups(ctx)
		stop()
		if err != nil {
			return pageViewState{}, err
		}
	}

	return state, nil
}

// loadPageComments loads page discussions only when the application feature is enabled.
func loadPageComments(
	ctx context.Context,
	slug string,
	enabled bool,
	catalog pageViewCatalogService,
) ([]domain.PageComment, error) {
	if !enabled {
		return nil, nil
	}

	stop := measurePageStage(ctx, "comments")
	comments, err := catalog.PageComments(ctx, slug)
	stop()
	return comments, err
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

		data, err := viewDataUseCases.Load(r, views, "New page")
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
		allowed, err := canEditPagePaths(r.Context(), accessUseCases, user, originalSlug, destinationSlug)
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		if !allowed {
			httpresponse.Problem(w, http.StatusForbidden, "You do not have permission to edit this page path.")
			return
		}

		metadata, err := pageMetadataFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Page validation failed.", "", err) {
				return
			}

			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		input, err := pageSaveInput(r.Context(), r, templateUseCases, user, originalSlug, metadata)
		if err != nil {
			writePageProblem(views.logger, w, err)
			return
		}

		page, err := pageUseCases.Save(r.Context(), input)
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

// canEditPagePaths reports whether the user may edit every non-empty page path.
func canEditPagePaths(
	ctx context.Context,
	access pageAccessReader,
	user domain.User,
	paths ...string,
) (bool, error) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}

		allowed, err := access.CanEdit(ctx, user, path)
		if err != nil || !allowed {
			return allowed, err
		}
	}

	return true, nil
}

// pageSaveInput builds the service input for a parsed page form.
func pageSaveInput(
	ctx context.Context,
	r *http.Request,
	templates templateService,
	user domain.User,
	originalSlug string,
	metadata domain.PageMetadata,
) (service.PageSaveInput, error) {
	markdown := r.FormValue("markdown")
	if originalSlug == "" {
		resolved, err := resolvePageTemplateFields(ctx, r, templates, markdown)
		if err != nil {
			return service.PageSaveInput{}, err
		}
		markdown = resolved
	}

	return service.PageSaveInput{
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
		Properties:         pagePropertiesFromForm(r),
		Actor:              user,
	}, nil
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
