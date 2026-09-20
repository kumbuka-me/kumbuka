package endpoint

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// PagePermalink redirects an authenticated stable page identifier to its current path.
func PagePermalink(catalogUseCases pagePermalinkService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := permalinkPageID(r.PathValue("id"))
		if err != nil {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		slug, err := catalogUseCases.PageSlugByID(r.Context(), id)
		if err != nil {
			writePermalinkProblem(logger, w, err)
			return
		}

		// The target may change when a page is moved, so this redirect must not be cached permanently.
		http.Redirect(w, r, "/pages/"+slug, http.StatusFound)
	}
}

// permalinkPageID validates the stable numeric page identifier used in permalink routes.
func permalinkPageID(value string) (pageID int64, err error) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, strconv.ErrSyntax
	}

	return id, nil
}

// writePermalinkProblem translates stable page lookup failures.
func writePermalinkProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
