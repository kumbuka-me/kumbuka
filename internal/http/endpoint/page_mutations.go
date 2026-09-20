package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// SavePageForm creates or updates a page from the browser form.
func SavePageForm(
	editorSaveUseCases pageEditorSave,
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

		metadata, err := pageMetadataFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Page validation failed.", "", err) {
				return
			}

			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		input, err := pageEditorSaveInput(r, user, originalSlug, metadata)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		page, err := editorSaveUseCases.Execute(r.Context(), input)
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+page.Slug, http.StatusSeeOther)
	}
}

// pageEditorSaveInput builds the application input for a parsed page editor form.
func pageEditorSaveInput(
	r *http.Request,
	user domain.User,
	originalSlug string,
	metadata domain.PageMetadata,
) (apppages.EditorSaveInput, error) {
	expectedUpdatedAt, err := expectedPageUpdatedAt(r.FormValue("expected_updated_at"), originalSlug != "")
	if err != nil {
		return apppages.EditorSaveInput{}, err
	}

	templateID := int64(0)
	if originalSlug == "" {
		templateID, err = pageTemplateID(r.FormValue("template_id"))
		if err != nil {
			return apppages.EditorSaveInput{}, err
		}
	}

	return apppages.EditorSaveInput{
		Page: apppages.PageSaveInput{
			PreviousSlug:       originalSlug,
			ExpectedUpdatedAt:  expectedUpdatedAt,
			Slug:               r.FormValue("slug"),
			Title:              r.FormValue("title"),
			Icon:               r.FormValue("icon"),
			Language:           r.FormValue("language"),
			Markdown:           r.FormValue("markdown"),
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
		},
		TemplateID:     templateID,
		TemplateValues: pageTemplateValues(r),
	}, nil
}

// pageTemplateID validates an optional template identifier submitted by the editor.
func pageTemplateID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, &domain.ValidationError{Fields: []domain.FieldError{{
			Field: "template", Message: "Choose a valid page template.",
		}}}
	}
	return id, nil
}

// pageTemplateValues extracts transport fields without requiring the endpoint to load template metadata.
func pageTemplateValues(r *http.Request) map[string]string {
	values := make(map[string]string)
	for name := range r.Form {
		field, ok := strings.CutPrefix(name, "blueprint_")
		if !ok || field == "" {
			continue
		}
		values[field] = r.FormValue(name)
	}
	return values
}

// expectedPageUpdatedAt parses the immutable editor version token for an existing page.
func expectedPageUpdatedAt(value string, required bool) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		if required {
			return time.Time{}, &domain.ValidationError{Fields: []domain.FieldError{{
				Field:   "expected_updated_at",
				Message: "Reload the page before saving it.",
			}}}
		}

		return time.Time{}, nil
	}

	nanoseconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || nanoseconds <= 0 {
		return time.Time{}, &domain.ValidationError{Fields: []domain.FieldError{{
			Field:   "expected_updated_at",
			Message: "Reload the page before saving it.",
		}}}
	}

	return time.Unix(0, nanoseconds), nil
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
func FavoritePage(catalogUseCases visiblePageActions, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		value := r.PathValue("slug")

		slug, ok := strings.CutSuffix(value, "/favorite")
		if !ok {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		user, _ := auth.User(r)

		if err := catalogUseCases.SetFavoriteFor(r.Context(), user, slug, r.FormValue("on") != "false"); err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		http.Redirect(w, r, "/pages/"+slug, http.StatusSeeOther)
	}
}

// WatchPage updates the current user's page or subtree subscription.
func WatchPage(catalogUseCases visiblePageActions, views *webview.Views) http.HandlerFunc {
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

		if err := catalogUseCases.SetPageWatchFor(r.Context(), user, slug, scope); err != nil {
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

	if conflict, ok := errors.AsType[*domain.PageEditConflictError](err); ok {
		message := "This page changed while you were editing it. Your changes are still in the editor. Open the latest page in another tab to compare, then reload before saving."
		if conflict.CurrentRevision > 0 {
			message = "This page changed while you were editing it. Revision " + strconv.Itoa(conflict.CurrentRevision) + " is now current. Your changes are still in the editor. Open the latest page in another tab to compare, then reload before saving."
		}
		httpresponse.Problem(w, http.StatusConflict, message)
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

	case errors.Is(err, apppages.ErrDiscussionsDisabled):
		httpresponse.Problem(w,
			http.StatusForbidden,
			"Page discussions are disabled.",
		)

	case errors.Is(err, domain.ErrStaleSuggestion):
		httpresponse.Problem(w,
			http.StatusConflict,
			"This suggestion can no longer be applied because the page changed after it was created.",
		)

	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
