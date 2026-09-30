package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManagementMetricsDisabled(t *testing.T) {
	response := httptest.NewRecorder()
	newManagement(Config{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusNotFound, response.Code)
}
