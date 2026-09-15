package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChain(t *testing.T) {
	t.Parallel()

	var calls []string
	record := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name+":before")

				next.ServeHTTP(w, r)

				calls = append(calls, name+":after")
			})
		}
	}

	handler := Chain(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			calls = append(calls, "handler")
			w.WriteHeader(http.StatusNoContent)
		}),
		record("first"),
		record("second"),
		record("third"),
	)

	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, http.StatusNoContent, response.Code)
	assert.Equal(t, []string{
		"first:before",
		"second:before",
		"third:before",
		"handler",
		"third:after",
		"second:after",
		"first:after",
	}, calls)
}
