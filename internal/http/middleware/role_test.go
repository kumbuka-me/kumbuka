package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
)

func TestRequireRole(t *testing.T) {
	t.Parallel()

	t.Run("allows an accepted role", func(t *testing.T) {
		t.Parallel()

		handler := RequireRole("admin", "editor")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/edit/page", nil),
			domain.User{ID: 1, Role: "editor"},
		)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNoContent, response.Code)
	})

	t.Run("rejects a browser role with a JSON problem", func(t *testing.T) {
		t.Parallel()

		handler := RequireRole("admin")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		request := auth.WithUser(
			httptest.NewRequest(http.MethodGet, "/admin", nil),
			domain.User{ID: 1, Role: "viewer"},
		)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.JSONEq(t, `{"error":"Forbidden.","problems":{}}`, response.Body.String())
	})

	t.Run("rejects an API role with a JSON problem", func(t *testing.T) {
		t.Parallel()

		handler := RequireRole("admin")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		request := auth.WithUser(
			httptest.NewRequest(http.MethodDelete, "/api/pages/example", nil),
			domain.User{ID: 1, Role: "editor"},
		)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusForbidden, response.Code)
		assert.JSONEq(
			t,
			`{"error":"Forbidden.","problems":{"authorization":"Your account does not have permission to perform this action."}}`,
			response.Body.String(),
		)
	})

	t.Run("rejects a missing authenticated user", func(t *testing.T) {
		t.Parallel()

		handler := RequireRole("admin")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			assert.Fail(t, "handler must not run")
		}))
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/admin", nil))

		assert.Equal(t, http.StatusUnauthorized, response.Code)
		assert.JSONEq(t, `{"error":"Unauthorized.","problems":{}}`, response.Body.String())
	})
}

func TestRequireRoleSnapshotsAllowedRoles(t *testing.T) {
	roles := []domain.UserRole{domain.UserRoleAdmin}
	middleware := RequireRole(roles...)
	roles[0] = "viewer"
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Run("keeps original role", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/admin", nil), domain.User{ID: 1, Role: "admin"})
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusNoContent, response.Code)
	})
	t.Run("ignores caller mutation", func(t *testing.T) {
		response := httptest.NewRecorder()
		request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/admin", nil), domain.User{ID: 1, Role: "viewer"})
		handler.ServeHTTP(response, request)
		assert.Equal(t, http.StatusForbidden, response.Code)
	})
}
