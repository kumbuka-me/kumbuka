package endpoint

import (
	"context"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// HealthProber verifies that a required application dependency is available.
type HealthProber interface {
	// Ping returns an error when the dependency is unavailable.
	Ping(context.Context) error
}

// Health serves the application's dependency health status.
func Health(prober HealthProber) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := prober.Ping(r.Context()); err != nil {
			httpresponse.Problem(w, http.StatusServiceUnavailable, "Database unavailable.")
			return
		}
		httpresponse.Respond(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
