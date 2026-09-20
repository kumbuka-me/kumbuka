package handler

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingOIDCIdentityID(t *testing.T) {
	t.Parallel()

	t.Run("accepts positive identifier", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("POST", "/admin/oidc/pending/42/link", nil)
		request.SetPathValue("id", "42")

		id, err := pendingOIDCIdentityID(request)

		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
	})

	t.Run("rejects invalid identifier", func(t *testing.T) {
		t.Parallel()

		request := httptest.NewRequest("POST", "/admin/oidc/pending/nope/link", nil)
		request.SetPathValue("id", "nope")

		_, err := pendingOIDCIdentityID(request)

		require.Error(t, err)
	})
}

type pendingIdentityStatusStub struct {
	oidcIdentityService
	id, actor int64
	rejected  bool
}

func (s *pendingIdentityStatusStub) SetPendingOIDCIdentityRejected(_ context.Context, id int64, rejected bool, actor int64) error {
	s.id, s.rejected, s.actor = id, rejected, actor
	return nil
}

func TestReopenPendingOIDCIdentity(t *testing.T) {
	users := &pendingIdentityStatusStub{rejected: true}
	mux := http.NewServeMux()

	mux.Handle("POST /admin/oidc/pending/{id}/reopen", ReopenPendingOIDCIdentity(users, slog.Default()))

	request := auth.WithUser(httptest.NewRequest("POST", "/admin/oidc/pending/42/reopen", nil), domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	mux.ServeHTTP(response, request)
	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/users#pending-identities", response.Header().Get("Location"))
	assert.Equal(t, int64(42), users.id)
	assert.Equal(t, int64(7), users.actor)
	assert.False(t, users.rejected)
}

type passwordUserStub struct{ input appusers.UserUpdateInput }

func (s *passwordUserStub) UpdateAccount(_ context.Context, input appusers.UserUpdateInput) error {
	s.input = input
	return nil
}

func TestUpdateAdminUserSubmitsCompleteAccountChange(t *testing.T) {
	users := &passwordUserStub{}
	form := url.Values{
		"role":                   {"admin"},
		"account_enabled":        {"on"},
		"local_password":         {"a-long-password-123"},
		"local_password_confirm": {"a-long-password-123"},
	}
	request := httptest.NewRequest("POST", "/admin/users/7", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("id", "7")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	UpdateAdminUser(users, testHandlerViews(t, webview.RuntimeInfo{}), slog.Default())(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, int64(7), users.input.UserID)
	assert.Equal(t, form.Get("local_password"), users.input.Password)
	assert.True(t, users.input.Enabled)
	assert.Equal(t, int64(7), users.input.Actor.ID)
}
