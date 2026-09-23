package endpoint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHTMLProblems verifies that selected browser JSON problems become themed status pages.
func TestHTMLProblems(t *testing.T) {
	t.Parallel()

	views := testHandlerViewsWithOverrides(
		t,
		testViewsLogger(),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/public_layout.gohtml": `{{ define "public-layout" }}{{ .StatusCode }}|{{ .Title }}|{{ .StatusMessage }}{{ end }}`,
		},
	)

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

	views := testHandlerViews(t, webview.RuntimeInfo{})
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

// TestHTMLProblemsRendersDocumentNavigationFailures verifies browser page errors never expose raw JSON.
func TestHTMLProblemsRendersDocumentNavigationFailures(t *testing.T) {
	t.Parallel()

	views := testHandlerViewsWithOverrides(
		t,
		testViewsLogger(),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/public_layout.gohtml": `{{ define "public-layout" }}{{ .StatusCode }}|{{ .Title }}|{{ .StatusMessage }}{{ end }}`,
		},
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(
			w,
			http.StatusInternalServerError,
			"The request could not be processed. Reference: test-reference",
		)
	})
	request := httptest.NewRequest(http.MethodGet, "/pages/example", nil)
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	response := httptest.NewRecorder()

	HTMLProblems(next, views).ServeHTTP(response, request)

	require.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "500|Something went wrong|")
	assert.Contains(t, response.Body.String(), "Reference: test-reference")
	assert.NotContains(t, response.Body.String(), `"error"`)
}

// TestHTMLProblemsLeavesJSONRequestsUntouched verifies API clients retain structured errors on browser paths.
func TestHTMLProblemsLeavesJSONRequestsUntouched(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{})
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusInternalServerError, "The request could not be processed.")
	})
	request := httptest.NewRequest(http.MethodGet, "/pages/example", nil)
	request.Header.Set("Accept", "application/json")
	response := httptest.NewRecorder()

	HTMLProblems(next, views).ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), `"error":"The request could not be processed."`)
}

// TestHTMLProblemsRendersPlainNavigationFailures verifies standard-library errors use the themed surface too.
func TestHTMLProblemsRendersPlainNavigationFailures(t *testing.T) {
	t.Parallel()

	views := testHandlerViewsWithOverrides(
		t,
		testViewsLogger(),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/public_layout.gohtml": `{{ define "public-layout" }}{{ .StatusCode }}|{{ .Title }}|{{ .StatusMessage }}{{ end }}`,
		},
	)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	})
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Set("Sec-Fetch-Mode", "navigate")
	response := httptest.NewRecorder()

	HTMLProblems(next, views).ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "404|Page not found|")
	assert.NotContains(t, response.Body.String(), "404 page not found")
}

// TestHTMLProblemsRendersThemedNotFound verifies hidden browser auth routes use the shared 404 page.
func TestHTMLProblemsRendersThemedNotFound(t *testing.T) {
	t.Parallel()

	views := testHandlerViewsWithOverrides(
		t,
		testViewsLogger(),
		webview.RuntimeInfo{},
		map[string]string{
			"templates/public_layout.gohtml": `{{ define "public-layout" }}{{ .StatusCode }}|{{ .Title }}|{{ .StatusMessage }}|{{ .StatusIcon }}|{{ .SecondaryURL }}{{ end }}`,
		},
	)

	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Problem(w, http.StatusNotFound, "Not found.")
	})

	request := httptest.NewRequest(http.MethodGet, "/auth/local", nil)
	response := httptest.NewRecorder()

	HTMLProblems(next, views, "/auth/local").ServeHTTP(response, request)

	require.Equal(t, http.StatusNotFound, response.Code)
	assert.Equal(t, "text/html; charset=utf-8", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Body.String(), "404|Page not found|")
	assert.Contains(t, response.Body.String(), "does not exist or may have moved")
	assert.Contains(t, response.Body.String(), "search-lucide|/search")
	assert.NotContains(t, response.Body.String(), `"error"`)
}

// TestHTMLProblemsPreservesSuccessfulCallback verifies that redirects and cookies pass through unchanged.
func TestHTMLProblemsPreservesSuccessfulCallback(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{})
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
