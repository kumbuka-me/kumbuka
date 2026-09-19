package handler

import (
	"log/slog"
	"mime/multipart"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/kumbuka-me/kumbuka/internal/importer"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

const (
	importMultipartMemory = 32 << 20
	importRequestOverhead = 1 << 20
	maxImportRequestBytes = importer.MaxBytes + importRequestOverhead
)

// AdminImport renders the import workspace.
func AdminImport(viewDataUseCases viewDataService, views *webview.Views) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := administrationData(r, viewDataUseCases, views, "Import", "import")
		if err != nil {
			httpresponse.InternalServerError(views.Logger(), w, err)
			return
		}

		data.Query = r.URL.Query().Get("result")
		views.Render(w, "admin_import", data)
	}
}

// ImportPages imports uploaded files using the explicitly selected source format.
func ImportPages(pageUseCases pageImportService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := currentUser(r)

		r.Body = http.MaxBytesReader(w, r.Body, maxImportRequestBytes)
		if err := r.ParseMultipartForm(importMultipartMemory); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Import is too large or invalid.")
			return
		}

		format, err := importer.ParseFormat(r.FormValue("format"))
		if err != nil {
			writeImportProblem(logger, w, "format", "", err)
			return
		}

		candidates, uploadErr := importUploadedFiles(r.MultipartForm.File["files"], format)
		if uploadErr != nil {
			writeImportProblem(logger, w, "files", uploadErr.filename, uploadErr.cause)
			return
		}
		if len(candidates) == 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Choose at least one supported import file.")
			return
		}

		imported, err := pageUseCases.Import(
			r.Context(),
			serviceImportPages(candidates),
			string(format),
			user,
		)
		if err != nil {
			writePageProblem(logger, w, err)
			return
		}

		http.Redirect(w, r, "/admin/import?result="+strconv.Itoa(imported), http.StatusSeeOther)
	}
}

// importUploadError associates an import failure with the uploaded file that caused it.
type importUploadError struct {
	// filename is the user-visible basename of the uploaded file.
	filename string
	// cause is the parser or I/O failure raised while processing the file.
	cause error
}

// importUploadedFiles parses all uploaded files against one shared uncompressed-size budget.
func importUploadedFiles(headers []*multipart.FileHeader, format importer.Format) ([]importer.Candidate, *importUploadError) {
	budget := importer.NewBudget(importer.MaxBytes)
	var candidates []importer.Candidate

	for _, header := range headers {
		items, err := importUploadedFile(header, format, budget)
		if err != nil {
			return nil, &importUploadError{
				filename: importFilename(header.Filename),
				cause:    err,
			}
		}

		candidates = append(candidates, items...)
	}

	return candidates, nil
}

// importUploadedFile opens one multipart file and delegates source parsing to the importer package.
func importUploadedFile(header *multipart.FileHeader, format importer.Format, budget *importer.Budget) ([]importer.Candidate, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}

	candidates, parseErr := importer.ParseFile(header.Filename, file, format, budget)
	closeErr := file.Close()

	if parseErr != nil {
		return nil, parseErr
	}
	if closeErr != nil {
		return nil, closeErr
	}

	return candidates, nil
}

// serviceImportPages maps parser output onto the page service import contract.
func serviceImportPages(candidates []importer.Candidate) []service.ImportedPage {
	pages := make([]service.ImportedPage, 0, len(candidates))

	for _, candidate := range candidates {
		pages = append(pages, service.ImportedPage{
			Slug:     candidate.Slug,
			Title:    candidate.Title,
			Markdown: candidate.Markdown,
			Source:   candidate.Source,
		})
	}

	return pages
}

// writeImportProblem writes safe importer validation failures and logs unexpected parser errors.
func writeImportProblem(logger *slog.Logger, w http.ResponseWriter, field, filename string, err error) {
	message, ok := userErrorMessage(err)
	if !ok {
		httpresponse.InternalServerError(logger, w, err)
		return
	}
	if filename != "" {
		message = filename + ": " + message
	}

	httpresponse.Problem(
		w,
		http.StatusBadRequest,
		"Import validation failed.",
		httpresponse.NewFieldProblem(field, message),
	)
}

// importFilename returns a stable user-visible basename for an uploaded multipart file.
func importFilename(name string) string {
	return path.Base(strings.ReplaceAll(name, "\\", "/"))
}
