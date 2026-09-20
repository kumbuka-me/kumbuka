package endpoint

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// statusPagePresentation describes the browser-facing content and actions for one HTTP status page.
type statusPagePresentation struct {
	// Title is the human-readable status heading.
	Title string
	// Message explains the status without exposing implementation details.
	Message string
	// Icon is the host icon rendered above the status code.
	Icon string
	// PrimaryLabel is the text of the primary action.
	PrimaryLabel string
	// PrimaryURL is the local target of the primary action.
	PrimaryURL string
	// PrimaryIcon is the host icon rendered in the primary action.
	PrimaryIcon string
	// SecondaryLabel is the text of the optional secondary action.
	SecondaryLabel string
	// SecondaryURL is the local target of the optional secondary action.
	SecondaryURL string
	// SecondaryIcon is the host icon rendered in the secondary action.
	SecondaryIcon string
}

// capturedResponse buffers one selected browser response so JSON problems can become themed HTML pages.
type capturedResponse struct {
	// header stores response headers written by the wrapped handler.
	header http.Header
	// body stores the response body written by the wrapped handler.
	body bytes.Buffer
	// status stores the explicit response status, or zero before any write.
	status int
}

// newCapturedResponse creates an empty response buffer.
func newCapturedResponse() *capturedResponse {
	return &capturedResponse{header: make(http.Header)}
}

// Header returns the buffered response headers.
func (r *capturedResponse) Header() http.Header {
	return r.header
}

// WriteHeader records the first HTTP status written by the wrapped handler.
func (r *capturedResponse) WriteHeader(status int) {
	if r.status != 0 {
		return
	}

	r.status = status
}

// Write records response bytes and supplies the implicit successful status when necessary.
func (r *capturedResponse) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}

	return r.body.Write(data)
}

// HTMLProblems converts JSON problem responses into themed HTML pages for selected browser routes.
func HTMLProblems(next http.Handler, views *webview.Views, paths ...string) http.Handler {
	selected := make(map[string]bool, len(paths))
	for _, path := range paths {
		selected[path] = true
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if views == nil || !selected[r.URL.Path] {
			next.ServeHTTP(w, r)
			return
		}

		captured := newCapturedResponse()
		next.ServeHTTP(captured, r)

		status := captured.status
		if status == 0 {
			status = http.StatusOK
		}
		if status < http.StatusBadRequest || !strings.HasPrefix(captured.header.Get("Content-Type"), "application/json") {
			writeCapturedResponse(w, captured, status)
			return
		}

		var problem struct {
			// Error is the safe problem message displayed on the browser error page.
			Error string `json:"error"`
		}
		if err := json.Unmarshal(captured.body.Bytes(), &problem); err != nil || strings.TrimSpace(problem.Error) == "" {
			writeCapturedResponse(w, captured, status)
			return
		}

		presentation := browserProblemPresentation(status, problem.Error)
		data, err := views.PublicData(presentation.Title)
		if err != nil {
			writeCapturedResponse(w, captured, status)
			return
		}

		copyCapturedHeaders(w.Header(), captured.header, true)
		w.Header().Set("Cache-Control", "no-store")
		renderStatusPage(views, w, status, "public-layout", data, presentation)
	})
}

// renderStatusPage renders the shared status surface with either the application or public layout.
func renderStatusPage(
	views *webview.Views,
	w http.ResponseWriter,
	status int,
	layout string,
	data webview.Layout,
	presentation statusPagePresentation,
) {
	data.Title = presentation.Title

	views.RenderDataStatus(w, status, "not_found", layout, webview.StatusView{
		Layout:         data,
		StatusCode:     status,
		StatusMessage:  presentation.Message,
		StatusIcon:     presentation.Icon,
		PrimaryLabel:   presentation.PrimaryLabel,
		PrimaryURL:     presentation.PrimaryURL,
		PrimaryIcon:    presentation.PrimaryIcon,
		SecondaryLabel: presentation.SecondaryLabel,
		SecondaryURL:   presentation.SecondaryURL,
		SecondaryIcon:  presentation.SecondaryIcon,
	})
}

// browserProblemPresentation maps stable browser authentication failures to friendly status content.
func browserProblemPresentation(status int, message string) statusPagePresentation {
	switch strings.TrimSpace(message) {
	case "Registration is closed. Your verified identity is awaiting administrator approval.":
		return statusPagePresentation{
			Title:        "Approval required",
			Message:      "Your identity was verified successfully, but your account must be approved by an administrator before you can sign in.",
			Icon:         "clock-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	case "This identity has been rejected by an administrator.":
		return statusPagePresentation{
			Title:        "Sign-in rejected",
			Message:      "This identity has been rejected by an administrator.",
			Icon:         "circle-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	case "User registration is disabled.":
		return statusPagePresentation{
			Title:        "Registration disabled",
			Message:      "New accounts cannot be created at this time.",
			Icon:         "circle-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	case "This account is disabled.":
		return statusPagePresentation{
			Title:        "Account disabled",
			Message:      "This account has been disabled. Contact an administrator if you need access.",
			Icon:         "circle-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	case "The preferred username is already used by another account.":
		return statusPagePresentation{
			Title:        "Username unavailable",
			Message:      "The username provided by your identity provider is already used by another account.",
			Icon:         "circle-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	case "Invalid login state.":
		return retrySignInPresentation(
			"Sign-in expired",
			"This sign-in request is no longer valid. Start the sign-in process again.",
		)
	case "Login failed.",
		"Invalid identity.",
		"Invalid claims.",
		"Invalid group claim.",
		"The preferred_username claim is required.":
		return retrySignInPresentation(
			"Sign-in failed",
			"Your identity provider could not complete the sign-in request. Please try again.",
		)
	case "OIDC authentication is not enabled.":
		return statusPagePresentation{
			Title:        "Sign-in unavailable",
			Message:      "External sign-in is not currently available.",
			Icon:         "circle-alert-lucide",
			PrimaryLabel: "Return home",
			PrimaryURL:   "/",
			PrimaryIcon:  "house-lucide",
		}
	}

	return genericStatusPresentation(status, message)
}

// notFoundPresentation returns the shared themed presentation for browser 404 responses.
func notFoundPresentation() statusPagePresentation {
	return statusPagePresentation{
		Title:          "Page not found",
		Message:        "The page you are looking for does not exist or may have moved.",
		Icon:           "search-lucide",
		PrimaryLabel:   "Return home",
		PrimaryURL:     "/",
		PrimaryIcon:    "house-lucide",
		SecondaryLabel: "Search pages",
		SecondaryURL:   "/search",
		SecondaryIcon:  "search-lucide",
	}
}

// retrySignInPresentation creates a retryable authentication status presentation.
func retrySignInPresentation(title, message string) statusPagePresentation {
	return statusPagePresentation{
		Title:          title,
		Message:        message,
		Icon:           "circle-alert-lucide",
		PrimaryLabel:   "Try again",
		PrimaryURL:     "/auth/login",
		PrimaryIcon:    "log-in-lucide",
		SecondaryLabel: "Return home",
		SecondaryURL:   "/",
		SecondaryIcon:  "house-lucide",
	}
}

// genericStatusPresentation creates a safe fallback presentation for an unmapped browser problem.
func genericStatusPresentation(status int, message string) statusPagePresentation {
	if status == http.StatusNotFound {
		return notFoundPresentation()
	}

	presentation := statusPagePresentation{
		Title:        http.StatusText(status),
		Message:      strings.TrimSpace(message),
		Icon:         "circle-alert-lucide",
		PrimaryLabel: "Return home",
		PrimaryURL:   "/",
		PrimaryIcon:  "house-lucide",
	}

	if presentation.Title == "" {
		presentation.Title = "Request failed"
	}
	if presentation.Message == "" {
		presentation.Message = "The request could not be completed."
	}
	if status >= http.StatusInternalServerError {
		presentation.Title = "Something went wrong"
		presentation.Message = "The request could not be processed. Please try again."
	}

	return presentation
}

// writeCapturedResponse copies an unmodified buffered response to the real response writer.
func writeCapturedResponse(w http.ResponseWriter, captured *capturedResponse, status int) {
	copyCapturedHeaders(w.Header(), captured.header, false)
	w.WriteHeader(status)
	_, _ = w.Write(captured.body.Bytes())
}

// copyCapturedHeaders copies buffered headers and optionally drops body-specific metadata before HTML rendering.
func copyCapturedHeaders(destination, source http.Header, dropBodyMetadata bool) {
	for key, values := range source {
		if dropBodyMetadata && (strings.EqualFold(key, "Content-Type") || strings.EqualFold(key, "Content-Length")) {
			continue
		}

		for _, value := range values {
			destination.Add(key, value)
		}
	}
}
