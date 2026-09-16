package handler

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// LocalLogin renders and processes the optional Kumbuka-managed sign-in flow.
func LocalLogin(
	settingsUseCases settingsService,
	systemUseCases systemService,
	browserAuth auth.BrowserAuth,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, err := browserAuth.LocalLoginAllowed(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		if !allowed {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		if views.runtime.AuthModeOverride == "" {
			settings, err := settingsUseCases.ApplicationSettings(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}

			required, err := systemUseCases.SetupRequired(r.Context())
			if err != nil {
				httpresponse.InternalServerError(views.logger, w, err)
				return
			}
			if required && settings.Authentication.Mode == string(auth.AuthModeNone) {
				http.Redirect(w, r, "/setup", http.StatusFound)
				return
			}
		}

		next := safeAuthNext(r.URL.Query().Get("next"))
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				httpresponse.Problem(w, http.StatusBadRequest, "Invalid login form.")
				return
			}

			next = safeAuthNext(r.FormValue("next"))
			_, token, err := browserAuth.Local.SignIn(r.Context(), r.FormValue("username"), r.FormValue("password"))
			if err == nil {
				browserAuth.Local.WriteSessionCookie(w, token)
				http.Redirect(w, r, cmp.Or(next, "/"), http.StatusSeeOther)
				return
			}
			writeLocalLoginProblem(views, w, err, next)
			return
		}

		data, err := publicViewData(views, "Local sign in")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		data.AuthNext = next

		renderPublic(views, w, "login", data)
	}
}

// Setup renders and processes the one-time first-administrator bootstrap.
func Setup(
	settingsUseCases settingsService,
	systemUseCases systemService,
	browserAuth auth.BrowserAuth,
	views *Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// An explicit runtime authentication override is itself a bootstrap or
		// recovery choice, so do not expose the unauthenticated setup surface.
		if views.runtime.AuthModeOverride != "" {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		if settings.Authentication.Mode != string(auth.AuthModeNone) {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		required, err := systemUseCases.SetupRequired(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}
		if !required {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				httpresponse.Problem(w, http.StatusBadRequest, "Invalid setup form.")
				return
			}

			problems := setupValidationProblems(r)
			if len(problems) > 0 {
				if wantsJSON(r) {
					httpresponse.Problem(w, http.StatusUnprocessableEntity, "Setup validation failed.", problems...)
					return
				}

				data, dataErr := publicViewData(views, "Set up Kumbuka")
				if dataErr != nil {
					httpresponse.InternalServerError(views.logger, w, dataErr)
					return
				}

				data.AuthError = problems[0].Message

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnprocessableEntity)
				renderPublic(views, w, "setup", data)
				return
			}

			user, token, err := browserAuth.Local.Setup(
				r.Context(),
				r.FormValue("username"),
				r.FormValue("email"),
				r.FormValue("display_name"),
				r.FormValue("password"),
			)
			if err == nil {
				browserAuth.Local.WriteSessionCookie(w, token)
				systemUseCases.RecordSetupCompleted(r.Context(), user)
				http.Redirect(w, r, "/admin/configuration", http.StatusSeeOther)
				return
			}
			writeSetupProblem(views, w, err)
			return
		}

		data, err := publicViewData(views, "Set up Kumbuka")
		if err != nil {
			httpresponse.InternalServerError(views.logger, w, err)
			return
		}

		renderPublic(views, w, "setup", data)
	}
}

// safeAuthNext accepts only local paths as post-authentication destinations.
func safeAuthNext(value string) string {
	value = strings.TrimSpace(value)
	if !httpresponse.IsLocalPath(value) {
		return ""
	}

	return value
}

// writeLocalLoginProblem preserves the sign-in form for invalid credentials.
func writeLocalLoginProblem(views *Views, w http.ResponseWriter, err error, next string) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		data, dataErr := publicViewData(views, "Local sign in")
		if dataErr != nil {
			httpresponse.InternalServerError(views.logger, w, dataErr)
			return
		}
		data.AuthError = "Invalid username or password."
		data.AuthNext = next
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		renderPublic(views, w, "login", data)
	default:
		httpresponse.InternalServerError(views.logger, w, err)
	}
}

// writeSetupProblem keeps an already-completed setup unavailable to anonymous callers.
func writeSetupProblem(views *Views, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAlreadyExists), errors.Is(err, domain.ErrForbidden):
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	default:
		httpresponse.InternalServerError(views.logger, w, err)
	}
}
