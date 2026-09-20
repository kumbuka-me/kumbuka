package endpoint

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadExportParameters(t *testing.T) {
	t.Parallel()

	t.Run("preserves explicit empty values", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"parameters":{"me.kumbuka.variables":{"values":{"environment":""}}}}`))
		values, err := readExportParameters(httptest.NewRecorder(), request)
		require.NoError(t, err)
		assert.Equal(t, "", values["me.kumbuka.variables"]["values"]["environment"])
	})

	t.Run("ignores bodies on get", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequest(http.MethodGet, "/", strings.NewReader(`{"parameters":{"me.kumbuka.variables":{"values":{"environment":"staging"}}}}`))
		values, err := readExportParameters(httptest.NewRecorder(), request)
		require.NoError(t, err)
		assert.Nil(t, values)
	})

	t.Run("rejects unknown request fields", func(t *testing.T) {
		t.Parallel()
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"parameters":{},"save":true}`))
		_, err := readExportParameters(httptest.NewRecorder(), request)
		require.Error(t, err)
	})
}

func TestValidateExportParameters(t *testing.T) {
	t.Parallel()

	assert.NoError(t, validateExportParameters(map[string]map[string]map[string]string{
		"me.kumbuka.variables": {"values": {"environment": " staging \n"}},
	}))

	err := validateExportParameters(map[string]map[string]map[string]string{
		"me.kumbuka.variables": {"values": {" environment ": "staging"}},
	})
	require.Error(t, err)
}
