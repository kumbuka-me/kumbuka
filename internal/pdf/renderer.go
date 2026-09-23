// Package pdf sends standalone page documents to an external PDF renderer.
package pdf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	maxHTMLBytes = 32 << 20
	maxPDFBytes  = 64 << 20
)

var (
	// ErrNotConfigured indicates PDF exports have no configured rendering endpoint.
	ErrNotConfigured = errors.New("render PDF: service is not configured")
	errInvalidURL    = &urlValidationError{}
)

// urlValidationError marks PDF endpoint validation text as safe for user presentation.
type urlValidationError struct{}

// Error returns the internal diagnostic error text.
func (*urlValidationError) Error() string {
	return "validate PDF URL: invalid endpoint"
}

// UserMessage returns the message explicitly approved for presentation to a user.
func (*urlValidationError) UserMessage() string {
	return "PDF URL must be an HTTP(S) endpoint including its path, without credentials or a fragment (for example http://html2pdf:8080/render)."
}

var renderClient = &http.Client{
	Timeout: 60 * time.Second,
	// Never forward private page content to a redirect target or change POST to GET.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// ValidateURL accepts an optional, complete HTTP endpoint including its path.
func ValidateURL(value string) error {
	if value == "" {
		return nil
	}

	endpoint, err := url.Parse(value)
	if err != nil {
		return errInvalidURL
	}
	if !validRenderEndpoint(endpoint) {
		return errInvalidURL
	}

	return nil
}

// validRenderEndpoint reports whether a parsed URL is safe and complete for PDF rendering.
func validRenderEndpoint(endpoint *url.URL) bool {
	if endpoint.Hostname() == "" || endpoint.Path == "" {
		return false
	}
	if endpoint.Scheme != "http" && endpoint.Scheme != "https" {
		return false
	}
	if endpoint.User != nil || endpoint.Fragment != "" {
		return false
	}

	return true
}

// Render POSTs HTML to endpoint exactly as configured and returns a temporary PDF. The caller must call cleanup after serving the file.
func Render(ctx context.Context, endpoint, title, language, rendered string, headers http.Header) (file *os.File, cleanup func(), err error) {
	noop := func() {}
	if endpoint == "" {
		return nil, noop, ErrNotConfigured
	}
	if err := ValidateURL(endpoint); err != nil {
		return nil, noop, err
	}

	request, err := newRenderRequest(ctx, endpoint, Document(title, language, rendered), headers)
	if err != nil {
		return nil, noop, err
	}
	response, err := renderClient.Do(request)
	if err != nil {
		return nil, noop, fmt.Errorf("request PDF service: %w", err)
	}
	defer response.Body.Close() // nolint:errcheck

	prefix, err := validateRenderResponse(response)
	if err != nil {
		return nil, noop, err
	}
	return writeTemporaryPDF(response.Body, prefix)
}

// newRenderRequest builds the bounded renderer request and applies caller headers before protocol headers.
func newRenderRequest(ctx context.Context, endpoint, content string, headers http.Header) (*http.Request, error) {
	if len(content) > maxHTMLBytes {
		return nil, errors.New("render PDF: document exceeds the 32 MiB request limit")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(content))
	if err != nil {
		return nil, err
	}
	for name, values := range headers {
		for _, value := range values {
			request.Header.Add(name, value)
		}
	}

	// Kumbuka owns the renderer protocol headers even when custom headers are configured.
	request.Header.Set("Content-Type", "text/html; charset=utf-8")
	request.Header.Set("Accept", "application/pdf")
	return request, nil
}

// validateRenderResponse validates status, media type, declared size, and the PDF signature.
func validateRenderResponse(response *http.Response) ([]byte, error) {
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("render PDF: service returned HTTP %d", response.StatusCode)
	}
	contentType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || contentType != "application/pdf" {
		return nil, errors.New("render PDF: service returned an unexpected content type")
	}
	if response.ContentLength > maxPDFBytes {
		return nil, errors.New("render PDF: service response exceeds the 64 MiB limit")
	}

	prefix := make([]byte, 5)
	if _, err := io.ReadFull(response.Body, prefix); err != nil || string(prefix) != "%PDF-" {
		return nil, errors.New("render PDF: service returned an invalid PDF")
	}
	return prefix, nil
}

// writeTemporaryPDF copies a bounded validated response into a rewound temporary file.
func writeTemporaryPDF(body io.Reader, prefix []byte) (file *os.File, cleanup func(), err error) {
	noop := func() {}
	file, err = os.CreateTemp("", "kumbuka-pdf-*.pdf")
	if err != nil {
		return nil, noop, err
	}
	cleanup = func() { _ = file.Close(); _ = os.Remove(file.Name()) }

	size, err := io.Copy(file, io.LimitReader(io.MultiReader(bytes.NewReader(prefix), body), maxPDFBytes+1))
	if err != nil {
		cleanup()
		return nil, noop, fmt.Errorf("read PDF service response: %w", err)
	}
	if size > maxPDFBytes {
		cleanup()
		return nil, noop, errors.New("render PDF: service response exceeds the 64 MiB limit")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, noop, err
	}
	return file, cleanup, nil
}

// Document wraps rendered page HTML in a self-contained print-oriented document. The caller must sanitize rendered HTML before passing it to this function.
func Document(title, language, rendered string) string {
	rendered = strings.ReplaceAll(rendered, " markdown-tab-panel-hidden", "")
	rendered = strings.ReplaceAll(
		rendered,
		`<details class="markdown-details"`,
		`<div class="markdown-details"`,
	)
	rendered = strings.ReplaceAll(rendered, "</details>", "</div>")
	rendered = strings.ReplaceAll(
		rendered,
		"<summary>",
		`<div class="markdown-details-summary">`,
	)
	rendered = strings.ReplaceAll(rendered, "</summary>", "</div>")

	title = html.EscapeString(title)
	language = html.EscapeString(language)

	return `<!doctype html>
<html lang="` + language + `">
<head>
	<meta charset="utf-8">
	<title>` + title + `</title>
	<style>` + styles + `</style>
</head>
<body>
	<main>
		<h1 class="document-title">` + title + `</h1>
		<article>` + rendered + `</article>
	</main>
</body>
</html>`
}
