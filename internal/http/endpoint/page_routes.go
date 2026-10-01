package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/route"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/pageurl"
)

// CanonicalPage renders a canonical stable-ID page URL and redirects stale or short forms.
func CanonicalPage(pages canonicalPageService, logger *slog.Logger, render http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page, err := canonicalPageForRequest(r, pages)
		if err != nil {
			writeCanonicalPageProblem(logger, w, err)
			return
		}
		if requestedSlug := strings.Trim(r.PathValue("slug"), "/"); requestedSlug != page.Slug {
			route.Redirect(w, r, pageurl.Page(page.ID, page.Slug), http.StatusPermanentRedirect)
			return
		}
		r.SetPathValue("slug", page.Slug)
		render.ServeHTTP(w, r)
	}
}

// LegacyPage redirects a current or historical slug directly to its canonical stable-ID URL.
func LegacyPage(pages canonicalPageService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, _ := auth.User(r)
		page, _, err := pages.GetPageOrAliasFor(r.Context(), user, r.PathValue("slug"))
		if err != nil {
			writeCanonicalPageProblem(logger, w, err)
			return
		}
		route.Redirect(w, r, pageurl.Page(page.ID, page.Slug), http.StatusPermanentRedirect)
	}
}

// redirectToCanonicalPage resolves a slug after a mutation and redirects directly to its stable-ID URL.
func redirectToCanonicalPage(w http.ResponseWriter, r *http.Request, pages canonicalPageService, logger *slog.Logger, slug string, status int) {
	user, _ := auth.User(r)
	page, _, err := pages.GetPageOrAliasFor(r.Context(), user, slug)
	if err != nil {
		writeCanonicalPageProblem(logger, w, err)
		return
	}
	route.Redirect(w, r, pageurl.Page(page.ID, page.Slug), status)
}

func canonicalPageForRequest(r *http.Request, pages canonicalPageService) (domain.Page, error) {
	id, err := canonicalPageID(r.PathValue("id"))
	if err != nil {
		return domain.Page{}, domain.ErrNotFound
	}
	user, _ := auth.User(r)
	return pages.PageByIDFor(r.Context(), user, id)
}

func canonicalPageID(value string) (int64, error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}
	return id, nil
}

func writeCanonicalPageProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	if errors.Is(err, domain.ErrNotFound) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
		return
	}
	httpresponse.InternalServerError(logger, w, err)
}
