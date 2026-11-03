package endpoint

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	httpprefix "github.com/containeroo/httpkit/prefix"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/importer"
	"github.com/kumbuka-me/kumbuka/internal/portable"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// portableArchiveRestorer restores one validated Kumbuka archive transactionally.
type portableArchiveRestorer interface {
	Restore(context.Context, portable.Archive, domain.User) (int, error)
}

// ImportPagesWithPortableArchive extends the normal admin importer with Kumbuka portable archives.
func ImportPagesWithPortableArchive(
	pageUseCases pageImportService,
	portableUseCases portableArchiveRestorer,
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
		if !portableSelected && (len(headers) != 1 || !strings.EqualFold(path.Ext(headers[0].Filename), ".zip")) {
			legacy.ServeHTTP(w, r)
			return
		}

		if len(headers) != 1 {
			httpresponse.Problem(w,
				http.StatusBadRequest,
				"Import validation failed.",
				httpresponse.NewFieldProblem("files", "Choose exactly one Kumbuka export ZIP."),
			)
			return
		}

		data, err := readPortableArchiveUpload(headers[0])
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		if !portableSelected && !portable.Detect(data, importer.MaxBytes) {
			legacy.ServeHTTP(w, r)
			return
		}
		archive, err := portable.Parse(data, importer.MaxBytes)
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		imported, err := portableUseCases.Restore(r.Context(), archive, currentUser(r))
		if err != nil {
			writePortableArchiveImportProblem(logger, w, err)
			return
		}

		httpprefix.Redirect(w, r, "/admin/import?result="+strconv.Itoa(imported), http.StatusSeeOther)
	}
}

// readPortableArchiveUpload reads one uploaded ZIP within the compressed size limit.
func readPortableArchiveUpload(header *multipart.FileHeader) ([]byte, error) {
	if strings.ToLower(path.Ext(header.Filename)) != ".zip" {
		return nil, newRequestError(
			"files",
			"Kumbuka imports require a .zip export archive.",
			errors.New("portable archive is not a ZIP"),
		)
	}

	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close() // nolint:errcheck

	data, err := io.ReadAll(io.LimitReader(file, importer.MaxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > importer.MaxBytes {
		return nil, newRequestError(
			"files",
			"Kumbuka archive exceeds 100 MiB.",
			errors.New("portable archive exceeds 100 MiB"),
		)
	}

	return data, nil
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
