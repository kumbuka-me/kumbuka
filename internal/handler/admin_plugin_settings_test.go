package handler

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdminPluginSettingsActionErrorReturnsFieldProblem verifies resource validation uses the shared form problem contract.
func TestAdminPluginSettingsActionErrorReturnsFieldProblem(t *testing.T) {
	t.Parallel()

	response, logs := pluginSettingsErrorResponse(t, &plugin.ConfigurationFieldError{
		Field:   "endpoint",
		Message: "API endpoint is required.",
	})

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.JSONEq(t, `{
		"error":"Plugin resource validation failed.",
		"problems":{"resource_endpoint":"API endpoint is required."}
	}`, response.Body.String())
	assert.Empty(t, logs)
}

// TestAdminPluginSettingsGroupErrorReturnsFieldProblem verifies typed settings map server validation to the submitted control.
func TestAdminPluginSettingsGroupErrorReturnsFieldProblem(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	views := testHandlerViewsWithLogger(t, slog.New(slog.NewTextHandler(&logs, nil)), webview.RuntimeInfo{})
	handler := &AdminPluginSettings{views: views}
	request := httptest.NewRequest(http.MethodPost, "/admin/plugin-settings/me.kumbuka.external-files/settings-group", bytes.NewBufferString("settings_id=appearance"))
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 1, Role: "admin", Enabled: true})
	response := httptest.NewRecorder()

	handler.writeActionError(response, request, "me.kumbuka.external-files", "settings-group", &plugin.ConfigurationFieldError{
		Field:   "reference_position",
		Message: "Reference position has an unsupported value.",
	})

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.JSONEq(t, `{
		"error":"Plugin settings validation failed.",
		"problems":{"setting_appearance_reference_position":"Reference position has an unsupported value."}
	}`, response.Body.String())
	assert.Empty(t, logs.String())
}

// TestAdminPluginSettingsActionErrorExplainsMissingEncryption verifies secret persistence failures are safe and actionable in the browser.
func TestAdminPluginSettingsActionErrorExplainsMissingEncryption(t *testing.T) {
	t.Parallel()

	response, logs := pluginSettingsErrorResponse(t, plugin.ErrSecretEncryptionUnavailable)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.JSONEq(t, `{
		"error":"Configure KUMBUKA__ENCRYPTION_KEY before saving plugin secrets.",
		"problems":{}
	}`, response.Body.String())
	assert.Contains(t, logs, "plugin.settings_encryption_required")
	assert.NotContains(t, logs, "plugin.settings_update_failed")
}

// pluginSettingsErrorResponse executes one JSON plugin-settings error response with an authenticated administrator.
func pluginSettingsErrorResponse(t *testing.T, err error) (*httptest.ResponseRecorder, string) {
	t.Helper()

	var logs bytes.Buffer
	views := testHandlerViewsWithLogger(t, slog.New(slog.NewTextHandler(&logs, nil)), webview.RuntimeInfo{})
	handler := &AdminPluginSettings{views: views}
	request := httptest.NewRequest(http.MethodPost, "/admin/plugin-settings/me.kumbuka.external-files/resource-save", nil)
	request.Header.Set("Accept", "application/json")
	request = auth.WithUser(request, domain.User{ID: 1, Role: "admin", Enabled: true})
	response := httptest.NewRecorder()

	handler.writeActionError(response, request, "me.kumbuka.external-files", "resource-save", err)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	return response, logs.String()
}
