package handler

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const maxBrandLogoRequestBytes = service.MaxBrandLogoBytes + (1 << 20)

// brandLogoService exposes the branding operations required by HTTP handlers.
type brandLogoService interface {
	BrandLogo(context.Context) (service.BrandLogo, error)
	SaveBrandLogo(context.Context, string, []byte, int64) error
	ClearBrandLogo(context.Context, int64) error
}

// AdminBranding renders instance-wide branding configuration.
func AdminBranding(viewDataUseCases viewDataService, views *Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Branding", "branding")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		render(views, w, "admin_branding", data)
	}
}

// BrandLogo serves the configured logo and falls back to the embedded favicon.svg.
func BrandLogo(
	settingsUseCases brandLogoService,
	appFS fs.FS,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if settingsUseCases != nil {
			logo, err := settingsUseCases.BrandLogo(r.Context())
			if err == nil {
				writeBrandLogo(w, logo.ContentType, logo.Data)
				return
			}
			if !errors.Is(err, domain.ErrNotFound) && logger != nil {
				logger.ErrorContext(
					r.Context(),
					"load brand logo",
					"event",
					"brand_logo_load_failed",
					"error",
					err,
				)
			}
		}

		data, err := fs.ReadFile(appFS, "favicon.svg")
		if err != nil {
			if logger == nil {
				httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		writeBrandLogo(w, "image/svg+xml", data)
	}
}

// SaveAdminBrandLogo validates and stores an administrator-provided logo file.
func SaveAdminBrandLogo(settingsUseCases brandLogoService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBrandLogoRequestBytes)
		if err := r.ParseMultipartForm(service.MaxBrandLogoBytes); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Logo upload is too large or invalid.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll() // nolint:errcheck
		}

		file, header, err := r.FormFile("logo")
		if err != nil {
			httpresponse.Problem(
				w,
				http.StatusBadRequest,
				"Brand logo validation failed.",
				httpresponse.NewFieldProblem("logo", "Choose a logo image."),
			)
			return
		}

		data, readErr := io.ReadAll(io.LimitReader(file, service.MaxBrandLogoBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			httpresponse.InternalServerError(logger, w, readErr)
			return
		}
		if closeErr != nil {
			httpresponse.InternalServerError(logger, w, closeErr)
			return
		}

		admin := currentUser(r)
		if err := settingsUseCases.SaveBrandLogo(r.Context(), header.Filename, data, admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "Brand logo")
			return
		}

		http.Redirect(w, r, "/admin/branding", http.StatusSeeOther)
	}
}

// ResetAdminBrandLogo removes the custom logo so favicon.svg is used again.
func ResetAdminBrandLogo(settingsUseCases brandLogoService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := settingsUseCases.ClearBrandLogo(r.Context(), admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "Brand logo")
			return
		}

		http.Redirect(w, r, "/admin/branding", http.StatusSeeOther)
	}
}

// writeBrandLogo writes one validated logo with conservative caching and SVG restrictions.
func writeBrandLogo(w http.ResponseWriter, contentType string, data []byte) {
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", contentType)
	if contentType == "image/svg+xml" {
		w.Header().Set(
			"Content-Security-Policy",
			"default-src 'none'; style-src 'unsafe-inline'; img-src data:; font-src data:; sandbox",
		)
	}

	_, _ = w.Write(data)
}
