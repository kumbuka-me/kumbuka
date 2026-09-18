package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// SavePageForm creates or updates a page from the browser form.
func SavePageForm(
	pageUseCases pageWriterService,
	draftUseCases draftDiscardService,
	templateUseCases templateService,
	accessUseCases pageAccessReader,
	views *webview.Views,
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
			httpresponse.InternalServerError(views.Logger(), w, err)
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

			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		input, err := pageSaveInput(r.Context(), r, templateUseCases, user, originalSlug, metadata)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		page, err := pageUseCases.Save(r.Context(), input)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		draftKey := "new"
		if originalSlug != "" {
			draftKey = service.PageDraftKey(page.ID)
		}

		if err := draftUseCases.Delete(r.Context(), user.ID, draftKey); err != nil {
			views.Logger().Warn(
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
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		slug := r.PathValue("slug")

		if err := pageUseCases.Delete(r.Context(), slug, user); err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
}

// FavoritePage updates the current user's favorite status for a page.
func FavoritePage(
	catalogUseCases favoriteService,
	views *webview.Views,
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
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// WatchPage updates the current user's page or subtree subscription.
func WatchPage(
	catalogUseCases pageWatchService,
	views *webview.Views,
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
			writePageProblem(views.Logger(), w, err)
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
