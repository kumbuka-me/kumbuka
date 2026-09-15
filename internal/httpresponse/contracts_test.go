package httpresponse

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These fixtures are also consumed by the TypeScript HTTP contract tests.
func TestProblemSharedContract(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../test/contracts/http.json")
	require.NoError(t, err)
	var fixtures map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &fixtures))
	t.Run("problem_without_fields", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		Problem(response, http.StatusBadRequest, "Page not found.")
		assert.JSONEq(t, string(fixtures["problem_without_fields"]), response.Body.String())
	})

	t.Run("problem_with_fields", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		Problem(response, http.StatusBadRequest, "Page validation failed.", NewFieldProblem("title", "Title is required."))
		assert.JSONEq(t, string(fixtures["problem_with_fields"]), response.Body.String())
	})
}
