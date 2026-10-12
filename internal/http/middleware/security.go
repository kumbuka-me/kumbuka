package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	httpkitmiddleware "github.com/containeroo/httpkit/middleware"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
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

	return httpkitmiddleware.SecurityHeaders(http.Header{
		"X-Content-Type-Options":  {"nosniff"},
		"X-Frame-Options":         {"DENY"},
		"Referrer-Policy":         {"same-origin"},
		"Content-Security-Policy": {cspValue},
	})
}

// RejectCrossSiteWrites blocks and logs cross-site state-changing browser requests.
func RejectCrossSiteWrites(logger *slog.Logger) Middleware {
	return httpkitmiddleware.RejectCrossOriginWrites(logger, func(w http.ResponseWriter, r *http.Request) {
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

// isCrossOriginWrite reports whether browser metadata identifies an unsafe request as originating outside this HTTP origin. Same-site is intentionally rejected because sibling origins can still receive SameSite cookies. Headerless non-browser clients remain allowed; Origin provides a fallback for browsers that do not send Fetch Metadata headers.
func isCrossOriginWrite(r *http.Request) bool {
	return httpkitmiddleware.IsCrossOriginWrite(r)
}

// isSafeMethod reports whether an HTTP method is defined as safe for cross-site requests.
func isSafeMethod(method string) bool {
	return httpkitmiddleware.IsSafeMethod(method)
}
