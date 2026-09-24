package endpoint

import (
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/importer"
	"github.com/kumbuka-me/kumbuka/internal/portable"
)

// portableArchivePageImportService combines legacy imports with portable archive page restoration.
type portableArchivePageImportService interface {
	pageImportService
	portablearchive.Pages
}

// ImportPagesWithPortableArchive extends the normal admin importer with Kumbuka portable archives.
func ImportPagesWithPortableArchive(
	pageUseCases portableArchivePageImportService,
	mediaUseCases portablearchive.Media,
	groupUseCases portablearchive.Groups,
	logger *slog.Logger,
) http.HandlerFunc {
	legacy := ImportPages(pageUseCases, logger)

	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxImportRequestBytes)
		if err := r.ParseMultipartForm(importMultipartMemory); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Import is too large or invalid.")
			return
		}
		if r.MultipartForm != nil {
			defer r.MultipartForm.RemoveAll() // nolint:errcheck
		}

		headers := r.MultipartForm.File["files"]
		portableSelected := strings.TrimSpace(r.FormValue("format")) == portable.Format
		if !portableSelected {
			detected, err := detectPortableArchiveUpload(headers)
			if err != nil {
				writePortableArchiveImportProblem(logger, w, err)
				return
			}
			if !detected {
				legacy.ServeHTTP(w, r)
				return
			}
		}

		if len(headers) != 1 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Import validation failed.",
				httpresponse.NewFieldProblem("files", "Choose exactly one Kumbuka export ZIP."),
			)
			return
		}

		archive, err := readPortableArchiveUpload(headers[0])
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		imported, err := portablearchive.Restore(
			r.Context(),
			archive,
			pageUseCases,
			mediaUseCases,
			groupUseCases,
			currentUser(r),
		)
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/admin/import?result="+strconv.Itoa(imported), http.StatusSeeOther)
	}
}

// detectPortableArchiveUpload reports whether one uploaded ZIP declares the Kumbuka portable format.
func detectPortableArchiveUpload(headers []*multipart.FileHeader) (bool, error) {
	if len(headers) != 1 || strings.ToLower(path.Ext(headers[0].Filename)) != ".zip" {
		return false, nil
	}

	file, err := headers[0].Open()
	if err != nil {
		return false, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, importer.MaxBytes+1))
	if err != nil {
		return false, err
	}
	if int64(len(data)) > importer.MaxBytes {
		return false, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return portable.Detect(data, importer.MaxBytes), nil
}

// readPortableArchiveUpload reads and validates one uploaded Kumbuka archive.
func readPortableArchiveUpload(header *multipart.FileHeader) (portable.Archive, error) {
	if strings.ToLower(path.Ext(header.Filename)) != ".zip" {
		return portable.Archive{}, newRequestError(
			"files",
			"Kumbuka imports require a .zip export archive.",
			errors.New("portable archive is not a ZIP"),
		)
	}

	file, err := header.Open()
	if err != nil {
		return portable.Archive{}, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, importer.MaxBytes+1))
	if err != nil {
		return portable.Archive{}, err
	}
	if int64(len(data)) > importer.MaxBytes {
		return portable.Archive{}, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return portable.Parse(data, importer.MaxBytes)
}

// writePortableArchiveImportProblem writes safe validation problems and logs unexpected restore failures.
func writePortableArchiveImportProblem(logger *slog.Logger, w http.ResponseWriter, err error) {
	var archiveValidation *portable.ValidationError
	if errors.As(err, &archiveValidation) {
		httpresponse.Problem(w,
			http.StatusBadRequest,
			"Import validation failed.",
			httpresponse.NewFieldProblem("files", archiveValidation.Message),
		)
		return
	}

	if message, ok := userErrorMessage(err); ok {
		httpresponse.Problem(w,
			http.StatusBadRequest,
			"Import validation failed.",
			httpresponse.NewFieldProblem("files", message),
		)
		return
	}

	if tryWriteValidationProblem(w, err, "Archive import failed.") {
		return
	}

	httpresponse.InternalServerError(logger, w, err)
}
