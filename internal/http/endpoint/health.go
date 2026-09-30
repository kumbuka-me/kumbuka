package endpoint

import (
	"context"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// ReadinessProber verifies that a required application dependency is available.
type ReadinessProber interface {
	// Ping returns an error when the dependency is unavailable.
	Ping(context.Context) error
}

// Health serves process liveness without depending on external services.
func Health() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		httpresponse.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// Ready serves application readiness and fails while a required dependency is unavailable.
func Ready(prober ReadinessProber) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := prober.Ping(r.Context()); err != nil {
			httpresponse.Problem(w, http.StatusServiceUnavailable, "Database unavailable.")
			return
		}
		httpresponse.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
