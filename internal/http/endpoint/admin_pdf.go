package endpoint

import (
	"cmp"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/pdf"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

// SaveAdminPDFSettings stores the database-managed PDF rendering endpoint and request headers.
func SaveAdminPDFSettings(settingsUseCases settingsService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		admin := currentUser(r)
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid PDF settings form.")
			return
		}

		pdfURL := strings.TrimSpace(r.FormValue("pdf_url"))
		if err := pdf.ValidateURL(pdfURL); err != nil {
			message, ok := userErrorMessage(err)
			if !ok {
				httpresponse.InternalServerError(logger, w, err)
				return
			}

			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"PDF settings validation failed.",
				httpresponse.NewFieldProblem("pdf_url", message),
			)
			return
		}

		headers, err := pdfHeadersFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid PDF settings form.", "pdf_headers", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}
		if err := settingsUseCases.SavePDFSettings(r.Context(), pdfURL, headers, admin.ID); err != nil {
			writeAdminProblem(logger, w, err, "PDF settings")
			return
		}

		http.Redirect(w, r, "/admin/configuration#pdf-rendering", http.StatusSeeOther)
	}
}

// TestAdminPDFService renders and returns Kumbuka's fixed two-page PDF diagnostic document.
func TestAdminPDFService(settingsUseCases settingsService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid PDF service test request.")
			return
		}

		pdfURL := strings.TrimSpace(r.FormValue("pdf_url"))
		if pdfURL == "" {
			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"PDF service test failed.",
				httpresponse.NewFieldProblem("pdf_url", "Enter a PDF service URL to test."),
			)
			return
		}
		if err := pdf.ValidateURL(pdfURL); err != nil {
			message, ok := userErrorMessage(err)
			if !ok {
				httpresponse.InternalServerError(logger, w, err)
				return
			}

			httpresponse.Problem(w,
				http.StatusUnprocessableEntity,
				"PDF service test failed.",
				httpresponse.NewFieldProblem("pdf_url", message),
			)
			return
		}

		inputs, err := pdfHeadersFromForm(r)
		if err != nil {
			if tryWriteRequestProblem(w, http.StatusBadRequest, "Invalid PDF service test request.", "pdf_headers", err) {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		headers, err := settingsUseCases.ResolvePDFRequestHeaders(r.Context(), inputs)
		if err != nil {
			if tryWriteValidationProblem(w, err, "PDF service test failed.") {
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		result, cleanup, err := pdf.RenderTest(r.Context(), pdfURL, pdfRequestHeaders(headers))
		if err != nil {
			logger.Warn("PDF service test failed", "event", "pdf_service_test_failed", "error", err)
			httpresponse.Problem(w,
				http.StatusBadGateway,
				"PDF service test failed.",
				httpresponse.NewFieldProblem("pdf_url", "The PDF service could not complete the test. Check the URL and service logs."),
			)
			return
		}
		defer cleanup()

		const filename = "kumbuka-pdf-service-test.pdf"

		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename=%q`, filename))
		w.Header().Set("X-Kumbuka-PDF-Pages", strconv.Itoa(result.PageCount))
		w.Header().Set("X-Kumbuka-PDF-Size", strconv.FormatInt(result.Size, 10))
		http.ServeContent(w, r, filename, time.Now(), result.File)
	}
}

// RevealAdminPDFHeader decrypts one sensitive PDF header after an explicit administrator action.
func RevealAdminPDFHeader(settingsUseCases settingsService, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "private, no-store")

		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid PDF header.")
			return
		}
		if id <= 0 {
			httpresponse.Problem(w, http.StatusBadRequest, "Invalid PDF header.")
			return
		}

		value, err := settingsUseCases.RevealPDFHeader(r.Context(), id)
		if err != nil {
			if tryWriteValidationProblem(w, err, "PDF header reveal failed.") {
				return
			}
			if errors.Is(err, domain.ErrNotFound) {
				httpresponse.Problem(w, http.StatusNotFound, "PDF header not found.")
				return
			}

			httpresponse.InternalServerError(logger, w, err)
			return
		}

		httpresponse.Respond(w, http.StatusOK, map[string]string{"value": value})
	}
}

// pdfHeadersFromForm parses the dynamic request-header rows in the PDF settings form.
func pdfHeadersFromForm(r *http.Request) ([]appsettings.PDFHeaderInput, error) {
	rows := r.Form["pdf_header_row"]
	if len(rows) == 0 {
		return nil, nil
	}

	seen := make(map[string]struct{}, len(rows))
	headers := make([]appsettings.PDFHeaderInput, 0, len(rows))
	for _, row := range rows {
		if !validDynamicFormRow(row) {
			return nil, newRequestError("pdf_headers", "The PDF header form is invalid.", nil)
		}
		if _, exists := seen[row]; exists {
			return nil, newRequestError("pdf_headers", "The PDF header form contains a duplicate row.", nil)
		}
		seen[row] = struct{}{}

		prefix := "pdf_header_" + row + "_"
		id := int64(0)
		if rawID := strings.TrimSpace(r.FormValue(prefix + "id")); rawID != "" {
			parsed, err := strconv.ParseInt(rawID, 10, 64)
			if err != nil {
				return nil, newRequestError("pdf_headers", "The PDF header form is invalid.", err)
			}
			if parsed <= 0 {
				return nil, newRequestError("pdf_headers", "The PDF header form is invalid.", nil)
			}
			id = parsed
		}

		headers = append(headers, appsettings.PDFHeaderInput{
			ID:        id,
			Name:      r.FormValue(prefix + "name"),
			Value:     r.FormValue(prefix + "value"),
			Sensitive: r.FormValue(prefix+"sensitive") == "on",
		})
	}

	return headers, nil
}

// pdfRequestHeaders converts resolved application settings into net/http request headers.
func pdfRequestHeaders(headers []domain.PDFHeader) http.Header {
	requestHeaders := make(http.Header, len(headers))
	for _, header := range headers {
		requestHeaders.Set(header.Name, header.Value)
	}

	return requestHeaders
}

// effectivePDFURL returns the deployment override when configured, otherwise the persisted endpoint.
func effectivePDFURL(runtimeOverride, persisted string) string {
	return cmp.Or(strings.TrimSpace(runtimeOverride), strings.TrimSpace(persisted))
}
