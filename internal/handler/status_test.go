package handler

import (
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTMLProblems verifies that selected browser JSON problems become themed status pages.
func TestHTMLProblems(t *testing.T) {
	t.Parallel()

	page := template.Must(template.New("not_found").Parse(
		`{{ define "public-layout" }}{{ .StatusCode }}|{{ .Title }}|{{ .StatusMessage }}{{ end }}`,
	))
	views := &Views{
		templates: map[string]*template.Template{"not_found": page},
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Set-Cookie", "kumbuka_state=; Max-Age=0; Path=/")
		httpresponse.Problem(
			w,
			http.StatusForbidden,
			"Registration is closed. Your verified identity is awaiting administrator approval.",
		)
	})

	request := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	response := httptest.NewRecorder()

	HTMLProblems(next, views, "/auth/callback").ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Header().Values("Set-Cookie"), "kumbuka_state=; Max-Age=0; Path=/")
	assert.Contains(t, response.Body.String(), "403|Approval required|")
	assert.Contains(t, response.Body.String(), "approved by an administrator")
	assert.NotContains(t, response.Body.String(), `"error"`)
}

// TestHTMLProblemsLeavesOtherRoutesUntouched verifies that unselected routes retain JSON problems.
func TestHTMLProblemsLeavesOtherRoutesUntouched(t *testing.T) {
	t.Parallel()

	views := &Views{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		httpresponse.Problem(w, http.StatusForbidden, "Forbidden.")
	})

	request := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	response := httptest.NewRecorder()

	HTMLProblems(next, views, "/auth/callback").ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
	assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), `"error":"Forbidden."`)
}

// TestHTMLProblemsPreservesSuccessfulCallback verifies that redirects and cookies pass through unchanged.
func TestHTMLProblemsPreservesSuccessfulCallback(t *testing.T) {
	t.Parallel()

	views := &Views{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "kumbuka_session", Value: "session", Path: "/"})
		http.Redirect(w, r, "/", http.StatusFound)
	})

	request := httptest.NewRequest(http.MethodGet, "/auth/callback", nil)
	response := httptest.NewRecorder()

	HTMLProblems(next, views, "/auth/callback").ServeHTTP(response, request)

	assert.Equal(t, http.StatusFound, response.Code)
	assert.Equal(t, "/", response.Header().Get("Location"))
	cookies := response.Result().Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "kumbuka_session", cookies[0].Name)
	assert.Equal(t, "session", cookies[0].Value)
}
