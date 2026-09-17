package handler

import (
	"cmp"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

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
