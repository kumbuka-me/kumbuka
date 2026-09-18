package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// Administration renders the administrator overview.
func Administration(
	viewDataUseCases viewDataService,
	administrationUseCases administrationService,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Administration", "overview")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		stats, err := administrationUseCases.Stats(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AdminStats = stats

		views.Render(w, "admin", data)
	}
}

// administrationData builds common view data for administrator-only pages.
func administrationData(
	r *http.Request,
	viewDataUseCases viewDataService,
	views *webview.Views,
	title, section string,
) (webview.Data, error) {
	data, err := viewDataUseCases.Load(r, views, title)
	if err != nil {
		return webview.Data{}, err
	}

	data.AdminSection = section
	data.Navigation = nil

	return data, nil
}

// writeAdminProblem translates expected administration errors into HTTP problems.
func writeAdminProblem(logger *slog.Logger, w http.ResponseWriter, err error, object string) {
	if tryWriteValidationProblem(w, err, object+" validation failed.") {
		return
	}
	switch {
	case errors.Is(err, domain.ErrNotFound):
		httpresponse.Problem(w, http.StatusNotFound, object+" not found.")
	case errors.Is(err, domain.ErrAlreadyExists):
		httpresponse.Problem(w, http.StatusConflict, object+" already exists.")
	case errors.Is(err, domain.ErrForbidden):
		httpresponse.Problem(w, http.StatusForbidden, object+" operation is not permitted.")
	default:
		httpresponse.InternalServerError(logger, w, err)
	}
}
