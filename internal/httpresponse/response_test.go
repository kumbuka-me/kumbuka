package httpresponse

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestProblemWritesStructuredJSON verifies the shared error contract used by handlers and middleware.
func TestProblemWritesStructuredJSON(t *testing.T) {
	t.Parallel()

	t.Run("writes field problems as an object", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		Problem(
			response,
			http.StatusUnprocessableEntity,
			"Page validation failed.",
			NewFieldProblem("slug", "A page path could not be derived."),
			NewFieldProblem("title", "Title is required."),
		)

		assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
		assert.Equal(t, "application/json; charset=utf-8", response.Header().Get("Content-Type"))
		assert.JSONEq(t, `{
			"error": "Page validation failed.",
			"problems": {
				"slug": "A page path could not be derived.",
				"title": "Title is required."
			}
		}`, response.Body.String())
	})

	t.Run("writes absent field problems as an empty object", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		Problem(response, http.StatusNotFound, "Page not found.")

		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.JSONEq(t, `{
			"error": "Page not found.",
			"problems": {}
		}`, response.Body.String())
	})
}

func TestXML(t *testing.T) {
	t.Parallel()

	t.Run("writes indented XML with declaration", func(t *testing.T) {
		t.Parallel()

		type document struct {
			XMLName xml.Name `xml:"root"`
			Value   string   `xml:"value"`
		}

		response := httptest.NewRecorder()

		err := XML(response, http.StatusCreated, document{Value: "example"})

		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, response.Code)
		assert.Equal(t, "application/xml; charset=utf-8", response.Header().Get("Content-Type"))
		assert.Equal(t, xml.Header+"<root>\n  <value>example</value>\n</root>\n", response.Body.String())
	})

	t.Run("returns marshal errors before writing", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()

		err := XML(response, http.StatusOK, struct {
			Value chan int `xml:"value"`
		}{Value: make(chan int)})

		assert.Error(t, err)
		assert.Empty(t, response.Header().Get("Content-Type"))
		assert.Empty(t, response.Body.String())
	})
}
