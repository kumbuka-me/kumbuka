package endpoint

import (
	"cmp"
	"errors"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// LocalLogin renders and processes the optional Kumbuka-managed sign-in flow.
func LocalLogin(
	settingsUseCases settingsService,
	systemUseCases systemService,
	browserAuth auth.BrowserAuth,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		allowed, err := browserAuth.LocalLoginAllowed(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		if !allowed {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		required, err := systemUseCases.SetupRequired(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		if required && settings.Authentication.Mode == domain.AuthModeNone {
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
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

		layout, err := views.PublicData("Local sign in")
		data := webview.AuthenticationView{Layout: layout}
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.AuthNext = next

		views.RenderPublic(w, "login", data)
	}
}

// Setup renders and processes the one-time first-administrator bootstrap.
func Setup(
	settingsUseCases settingsService,
	systemUseCases systemService,
	browserAuth auth.BrowserAuth,
	views *webview.Views,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		if settings.Authentication.Mode != domain.AuthModeNone {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		required, err := systemUseCases.SetupRequired(r.Context())
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}
		if !required {
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		}

		if r.Method == http.MethodPost {
			submitSetup(w, r, systemUseCases, browserAuth, views)
			return
		}

		renderSetupForm(w, views, http.StatusOK, "")
	}
}

// submitSetup validates and creates the initial administrator account.
func submitSetup(
	w http.ResponseWriter,
	r *http.Request,
	systemUseCases systemService,
	browserAuth auth.BrowserAuth,
	views *webview.Views,
) {
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

		renderSetupForm(w, views, http.StatusUnprocessableEntity, problems[0].Message)
		return
	}

	bootstrapSession := views.Runtime().AuthModeOverride != "" &&
		views.Runtime().AuthModeOverride != domain.AuthModeLocal

	var user domain.User
	var token string
	var err error
	if bootstrapSession {
		user, token, err = browserAuth.Local.SetupBootstrap(
			r.Context(),
			r.FormValue("username"),
			r.FormValue("email"),
			r.FormValue("display_name"),
			r.FormValue("password"),
		)
	} else {
		user, token, err = browserAuth.Local.Setup(
			r.Context(),
			r.FormValue("username"),
			r.FormValue("email"),
			r.FormValue("display_name"),
			r.FormValue("password"),
		)
	}
	if err != nil {
		writeSetupProblem(views, w, err)
		return
	}

	if bootstrapSession {
		browserAuth.Local.WriteBootstrapSessionCookie(w, token)
	} else {
		browserAuth.Local.WriteSessionCookie(w, token)
	}
	systemUseCases.RecordSetupCompleted(r.Context(), user)
	http.Redirect(w, r, "/admin/configuration", http.StatusSeeOther)
}

// renderSetupForm renders the setup form with the requested browser status.
func renderSetupForm(w http.ResponseWriter, views *webview.Views, status int, message string) {
	layout, err := views.PublicData("Set up Kumbuka")
	data := webview.AuthenticationView{Layout: layout}
	if err != nil {
		httpresponse.InternalServerError(views.Logger(), w, err)
		return
	}

	data.AuthError = message
	views.RenderPublicStatus(w, status, "setup", data)
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
func writeLocalLoginProblem(views *webview.Views, w http.ResponseWriter, err error, next string) {
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		layout, dataErr := views.PublicData("Local sign in")
		data := webview.AuthenticationView{Layout: layout}
		if dataErr != nil {
			httpresponse.InternalServerError(views.Logger(), w, dataErr)
			return
		}
		data.AuthError = "Invalid username or password."
		data.AuthNext = next
		views.RenderPublicStatus(w, http.StatusUnauthorized, "login", data)
	default:
		httpresponse.InternalServerError(views.Logger(), w, err)
	}
}

// writeSetupProblem keeps an already-completed setup unavailable to anonymous callers.
func writeSetupProblem(views *webview.Views, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrAlreadyExists), errors.Is(err, domain.ErrForbidden):
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	default:
		httpresponse.InternalServerError(views.Logger(), w, err)
	}
}
