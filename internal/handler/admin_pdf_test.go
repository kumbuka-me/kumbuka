package handler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pdfSettingsStub struct {
	settingsService
	url             string
	actorID         int64
	savedHeaders    []service.PDFHeaderInput
	resolvedHeaders []domain.PDFHeader
	resolvedInputs  []service.PDFHeaderInput
	revealedValue   string
}

func (s *pdfSettingsStub) SavePDFSettings(
	_ context.Context,
	pdfURL string,
	headers []service.PDFHeaderInput,
	actorID int64,
) error {
	s.url = pdfURL
	s.savedHeaders = headers
	s.actorID = actorID

	return nil
}

func (s *pdfSettingsStub) ResolvePDFRequestHeaders(
	_ context.Context,
	headers []service.PDFHeaderInput,
) ([]domain.PDFHeader, error) {
	s.resolvedInputs = headers
	return s.resolvedHeaders, nil
}

func (s *pdfSettingsStub) RevealPDFHeader(context.Context, int64) (string, error) {
	return s.revealedValue, nil
}

func TestEffectivePDFURL(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "http://runtime/render", effectivePDFURL("http://runtime/render", "http://saved/render"))
	assert.Equal(t, "http://saved/render", effectivePDFURL("", "http://saved/render"))
	assert.Empty(t, effectivePDFURL("", ""))
}

func TestSaveAdminPDFSettings(t *testing.T) {
	t.Parallel()

	settings := &pdfSettingsStub{}
	form := url.Values{
		"pdf_url":                 {" http://html2pdf:8080/render "},
		"pdf_header_row":          {"n1"},
		"pdf_header_n1_name":      {" X-Tenant "},
		"pdf_header_n1_value":     {" documentation "},
		"pdf_header_n1_sensitive": {"on"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/pdf", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	SaveAdminPDFSettings(settings, slog.Default())(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/configuration#pdf-rendering", response.Header().Get("Location"))
	assert.Equal(t, "http://html2pdf:8080/render", settings.url)
	assert.Equal(t, int64(7), settings.actorID)
	assert.Equal(t, []service.PDFHeaderInput{{Name: " X-Tenant ", Value: " documentation ", Sensitive: true}}, settings.savedHeaders)
}

func TestTestAdminPDFServiceUsesCurrentHeaders(t *testing.T) {
	t.Parallel()

	payload := `%PDF-1.7
1 0 obj
<</Type /Pages/Kids [2 0 R 3 0 R]/Count 2>>
endobj
2 0 obj
<</Type /Page/Parent 1 0 R>>
endobj
3 0 obj
<</Type /Page/Parent 1 0 R>>
endobj
%%EOF
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/render", r.URL.Path)
		assert.Equal(t, "Bearer secret-token", r.Header.Get("Authorization"))

		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		assert.Contains(t, string(body), "Kumbuka PDF service test")
		assert.Contains(t, string(body), "data:image/png;base64,")
		assert.Contains(t, string(body), "break-before: page")
		assert.Contains(t, string(body), "日本語")
		w.Header().Set("Content-Type", "application/pdf")

		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()

	settings := &pdfSettingsStub{resolvedHeaders: []domain.PDFHeader{{Name: "Authorization", Value: "Bearer secret-token"}}}
	form := url.Values{
		"pdf_url":                  {server.URL + "/render"},
		"pdf_header_row":           {"h12"},
		"pdf_header_h12_id":        {"12"},
		"pdf_header_h12_name":      {"Authorization"},
		"pdf_header_h12_value":     {""},
		"pdf_header_h12_sensitive": {"on"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/pdf/test", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	TestAdminPDFService(settings, slog.Default())(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "application/pdf", response.Header().Get("Content-Type"))
	assert.Equal(t, `inline; filename="kumbuka-pdf-service-test.pdf"`, response.Header().Get("Content-Disposition"))
	assert.Equal(t, "2", response.Header().Get("X-Kumbuka-PDF-Pages"))
	assert.Equal(t, strconv.Itoa(len(payload)), response.Header().Get("X-Kumbuka-PDF-Size"))
	assert.Equal(t, payload, response.Body.String())
	assert.Equal(t, []service.PDFHeaderInput{{ID: 12, Name: "Authorization", Sensitive: true}}, settings.resolvedInputs)
}

func TestRevealAdminPDFHeader(t *testing.T) {
	t.Parallel()

	settings := &pdfSettingsStub{revealedValue: "Bearer secret-token"}
	request := httptest.NewRequest(http.MethodPost, "/admin/pdf/headers/12/reveal", nil)
	request.SetPathValue("id", "12")
	response := httptest.NewRecorder()

	RevealAdminPDFHeader(settings, slog.Default())(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "private, no-store", response.Header().Get("Cache-Control"))
	assert.JSONEq(t, `{"value":"Bearer secret-token"}`, response.Body.String())
}

func TestSaveAdminPDFSettingsUsesTypedValidationMessage(t *testing.T) {
	t.Parallel()

	settings := &pdfSettingsStub{}
	form := url.Values{"pdf_url": {"ftp://example.test/render"}}
	request := httptest.NewRequest(http.MethodPost, "/admin/pdf", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	SaveAdminPDFSettings(settings, slog.New(slog.NewTextHandler(io.Discard, nil)))(response, request)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.Contains(t, response.Body.String(), "PDF URL must be an HTTP(S) endpoint")
	assert.NotContains(t, response.Body.String(), "validate PDF URL: invalid endpoint")
	assert.Empty(t, settings.url)
}

func TestAdminPDFServiceDoesNotExposeRendererError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	settings := &pdfSettingsStub{}
	form := url.Values{"pdf_url": {server.URL + "/render"}}
	request := httptest.NewRequest(http.MethodPost, "/admin/pdf/test", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	TestAdminPDFService(settings, slog.New(slog.NewTextHandler(io.Discard, nil)))(response, request)

	assert.Equal(t, http.StatusBadGateway, response.Code)
	assert.Contains(t, response.Body.String(), "The PDF service could not complete the test.")
	assert.NotContains(t, response.Body.String(), "render PDF: service returned HTTP 500")
}
