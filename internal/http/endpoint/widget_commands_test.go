package endpoint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRespondPluginWidgetCommand verifies in-place browser commands avoid navigation while regular widget forms keep redirect semantics.
func TestRespondPluginWidgetCommand(t *testing.T) {
	t.Parallel()

	t.Run("browser command returns JSON without redirecting", func(t *testing.T) {
		t.Parallel()

		request := widgetCommandResponseRequest(t, "response=json&next=%2Fpages%2Fexample")
		response := httptest.NewRecorder()

		respondPluginWidgetCommand(response, request, sdk.WidgetCommandResult{})

		require.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, response.Header().Get("Location"))
		var body pluginWidgetCommandResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Empty(t, body.Redirect)
	})

	t.Run("browser command preserves an explicit plugin redirect", func(t *testing.T) {
		t.Parallel()

		request := widgetCommandResponseRequest(t, "response=json&next=%2Fpages%2Fexample")
		response := httptest.NewRecorder()

		respondPluginWidgetCommand(response, request, sdk.WidgetCommandResult{Redirect: "/pages/other"})

		require.Equal(t, http.StatusOK, response.Code)
		assert.Empty(t, response.Header().Get("Location"))
		var body pluginWidgetCommandResponse
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, "/pages/other", body.Redirect)
	})

	t.Run("regular widget command prefers plugin redirect", func(t *testing.T) {
		t.Parallel()

		request := widgetCommandResponseRequest(t, "next=%2Fpages%2Fexample")
		response := httptest.NewRecorder()

		respondPluginWidgetCommand(response, request, sdk.WidgetCommandResult{Redirect: "/pages/other"})

		require.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, "/pages/other", response.Header().Get("Location"))
	})

	t.Run("regular widget command falls back to next", func(t *testing.T) {
		t.Parallel()

		request := widgetCommandResponseRequest(t, "next=%2Fpages%2Fexample")
		response := httptest.NewRecorder()

		respondPluginWidgetCommand(response, request, sdk.WidgetCommandResult{})

		require.Equal(t, http.StatusSeeOther, response.Code)
		assert.Equal(t, "/pages/example", response.Header().Get("Location"))
	})
}

// widgetCommandResponseRequest creates a parsed command request for response-contract tests.
func widgetCommandResponseRequest(t testing.TB, form string) *http.Request {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/plugins/actions/plugin/widget/action", strings.NewReader(form))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())
	return request
}
