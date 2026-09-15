package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeRequestContract(t *testing.T) {
	t.Parallel()
	type request struct {
		Title string `json:"title"`
	}
	t.Run("object", func(t *testing.T) {
		t.Parallel()

		result, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"Example"}`)))

		require.NoError(t, err)
		assert.Equal(t, "Example", result.Title)
	})

	t.Run("trailing whitespace", func(t *testing.T) {
		t.Parallel()

		result, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("{\"title\":\"Example\"} \n\t")))

		require.NoError(t, err)
		assert.Equal(t, "Example", result.Title)
	})

	t.Run("empty body", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("")))

		assert.Error(t, err)
	})

	t.Run("null", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("null")))

		assert.Error(t, err)
	})

	t.Run("array", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("[]")))

		assert.Error(t, err)
	})

	t.Run("unknown field", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":1}`)))

		assert.Error(t, err)
	})

	t.Run("wrong field type", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":42}`)))

		assert.Error(t, err)
	})

	t.Run("second object", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"Example"}{}`)))

		assert.Error(t, err)
	})

	t.Run("second null", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"Example"} null`)))

		assert.Error(t, err)
	})

	t.Run("trailing junk", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"Example"}oops`)))

		assert.Error(t, err)
	})

	t.Run("oversized value", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"`+strings.Repeat("a", 2<<20)+`"}`)))

		assert.Error(t, err)
	})

	t.Run("oversized trailing whitespace", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{}`+strings.Repeat(" ", 2<<20))))

		assert.Error(t, err)
	})
}

func TestDecodeReturnsSafeUserMessages(t *testing.T) {
	t.Parallel()

	type request struct {
		Title string `json:"title"`
	}

	t.Run("unknown field", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"unknown":1}`)))

		message, ok := userErrorMessage(err)
		require.True(t, ok)
		assert.Equal(t, "Request body contains an unknown field.", message)
		assert.Equal(t, `invalid JSON request: json: unknown field "unknown"`, err.Error())
		assert.NotContains(t, message, `"unknown"`)
	})

	t.Run("oversized body", func(t *testing.T) {
		t.Parallel()

		_, err := decode[request](httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader(`{"title":"`+strings.Repeat("a", 2<<20)+`"}`)))

		message, ok := userErrorMessage(err)
		require.True(t, ok)
		assert.Equal(t, "Request body must not exceed 2 MiB.", message)
	})
}

func TestJSONSlice(t *testing.T) {
	t.Parallel()

	t.Run("nil becomes an empty array", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []string{}, jsonSlice[string](nil))
	})

	t.Run("nonempty collection is preserved", func(t *testing.T) {
		t.Parallel()

		values := []string{"one"}
		assert.Equal(t, values, jsonSlice(values))
	})
}
