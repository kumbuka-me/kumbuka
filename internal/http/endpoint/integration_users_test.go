package endpoint

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
)

type integrationUserDirectoryStub struct {
	userDirectoryService
	user domain.User
}

// User returns the configured integration user.
func (s integrationUserDirectoryStub) User(context.Context, int64) (domain.User, error) {
	return s.user, nil
}

type integrationSettingsStub struct {
	settingsService
	enabled bool
}

// ApplicationSettings returns the configured integration-directory state.
func (s integrationSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return domain.ApplicationSettings{IntegrationUserDirectoryEnabled: s.enabled}, nil
}

// TestIntegrationUserRequiresAdministratorEnablement verifies contact data is disabled by default.
func TestIntegrationUserRequiresAdministratorEnablement(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	users := integrationUserDirectoryStub{user: domain.User{ID: 42, Username: "alice", DisplayName: "Alice", Email: "alice@example.test", Enabled: true}}

	disabled := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/integration/users/42", nil)
	request.SetPathValue("id", "42")
	IntegrationUser(users, integrationSettingsStub{}, logger)(disabled, request)
	assert.Equal(t, http.StatusNotFound, disabled.Code)

	enabled := httptest.NewRecorder()
	IntegrationUser(users, integrationSettingsStub{enabled: true}, logger)(enabled, request)
	assert.Equal(t, http.StatusOK, enabled.Code)
	assert.JSONEq(t, `{"id":42,"mention":"@alice","display_name":"Alice","email":"alice@example.test","enabled":true}`, enabled.Body.String())
}
