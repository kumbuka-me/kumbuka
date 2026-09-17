package middleware

import (
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// SecurityHeaders adds Kumbuka security response headers.
func SecurityHeaders() Middleware {
	cspDirectives := []string{
		"default-src 'self'",
		"script-src 'self' https://cdn.jsdelivr.net",
		"style-src 'self' 'unsafe-inline'",
		"img-src 'self' data: https:",
		"font-src 'self' https://cdn.jsdelivr.net",
		"connect-src 'self'",
		"frame-src 'self' blob:",
	}
	cspValue := strings.Join(cspDirectives, "; ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "same-origin")
			w.Header().Set("Content-Security-Policy", cspValue)

			next.ServeHTTP(w, r)
		})
	}
}

// RejectCrossSiteWrites blocks and logs cross-site state-changing browser requests.
func RejectCrossSiteWrites(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) || !isCrossOriginWrite(r) {
				next.ServeHTTP(w, r)
				return
			}

			logger.Warn(
				"cross-site write rejected",
				"event", "cross_site_write_rejected",
				"method", r.Method,
				"path", r.URL.Path,
				"origin", r.Header.Get("Origin"),
				"sec_fetch_site", r.Header.Get("Sec-Fetch-Site"),
				"remote_addr", r.RemoteAddr,
			)

			if strings.HasPrefix(r.URL.Path, "/api/") {
				httpresponse.Problem(w,
					http.StatusForbidden,
					"Forbidden.",
					httpresponse.NewFieldProblem(
						"request",
						"Cross-site write requests are not allowed.",
					),
				)
				return
			}

			httpresponse.Problem(w, http.StatusForbidden, "Forbidden.")
		})
	}
}

// isCrossOriginWrite reports whether browser metadata identifies an unsafe
// request as originating outside this HTTP origin. Same-site is intentionally
// rejected because sibling origins can still receive SameSite cookies.
// Headerless non-browser clients remain allowed; Origin provides a fallback
// for browsers that do not send Fetch Metadata headers.
func isCrossOriginWrite(r *http.Request) bool {
	switch strings.ToLower(strings.TrimSpace(r.Header.Get("Sec-Fetch-Site"))) {
	case "cross-site", "same-site":
		return true
	case "same-origin":
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return false
	}

	parsed, err := url.Parse(origin)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return true
	}

	return !strings.EqualFold(parsed.Host, r.Host)
}

// isSafeMethod reports whether an HTTP method is defined as safe for cross-site requests.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
