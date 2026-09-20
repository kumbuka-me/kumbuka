package middleware

import (
	"fmt"
	"log/slog"
	"net/http"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
)

// RecoverPanics converts handler panics into logged internal server errors.
func RecoverPanics(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if recovered == http.ErrAbortHandler {
						panic(recovered)
					}
					committed := false
					if response, ok := w.(*httpresponse.RequestWriter); ok {
						committed = response.Status != 0
					}
					httpresponse.InternalServerError(logger.With("operation", "request_panic"), w, fmt.Errorf("request panic: %v", recovered))
					if committed {
						panic(http.ErrAbortHandler)
					}
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
