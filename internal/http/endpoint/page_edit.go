package endpoint

import (
	"cmp"
	"errors"
	"net/http"
	"strconv"
	"strings"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// EditPage renders the page creation or editing form.
func EditPage(
	browserContext browserContextLoader,
	editorUseCases pageEditorQuery,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)
		slug := r.PathValue("slug")
		result, err := editorUseCases.Load(r.Context(), user, slug, selectedPageTemplateID(r.URL.Query().Get("template")))
		if errors.Is(err, domain.ErrNotFound) {
			renderNotFoundPage(w, r, browserContext, views)
			return
		}
		if err != nil {
			writePageProblem(views.Logger(), w, err)
			return
		}

		title := "New page"
		if result.Page != nil {
			title = "Edit " + result.Page.Title
		}
		layout, err := browserContext.Load(r, views, title)
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data := webview.EditView{Layout: layout}
		data.PageContentLanguage = layout.ApplicationSettings.ContentLanguage
		data.Groups = result.Groups
		data.ContentLanguages = contentLanguageOptions
		data.PageStatuses = domain.PageStatuses()

		if result.Page == nil {
			prepareNewPageEditor(r, &data, result.Templates, result.SelectedTemplate)
		} else {
			page := result.Page
			data.Title = title
			data.Page = page
			data.EditorInitialSlug = page.Slug
			data.EditorParentPath, data.EditorPathSegment = splitPagePath(page.Slug)
			data.PagePathOptions = webview.PagePathOptions(data.Navigation, page.Slug)
			data.PageContentLanguage = cmp.Or(page.Language, data.PageContentLanguage)
		}

		data.CurrentPage = webview.CurrentPage(data.Page)
		views.Render(w, "edit", data)
	}
}

// selectedPageTemplateID parses an optional new-page template selection and ignores invalid query values.
func selectedPageTemplateID(value string) int64 {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

// prepareNewPageEditor initializes presentation state used only when creating a page.
func prepareNewPageEditor(
	r *http.Request,
	data *webview.EditView,
	templates []domain.PageTemplate,
	selected *domain.PageTemplate,
) {
	data.PagePathOptions = webview.PagePathOptions(data.Navigation, "")

	prefillSlug := md.Slug(r.URL.Query().Get("slug"))

	switch prefillSlug {
	case "":
		parent := md.Slug(r.URL.Query().Get("parent"))
		if webview.HasPagePathOption(data.PagePathOptions, parent) {
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

	data.PageTemplates = templates
	data.EditorTemplate = selected
}

// ensurePagePathOption adds a path option when it does not already exist.
func ensurePagePathOption(options []webview.PagePathOption, slug string) []webview.PagePathOption {
	if slug == "" || webview.HasPagePathOption(options, slug) {
		return options
	}

	return append(options, webview.PagePathOption{
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
