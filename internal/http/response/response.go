// Package httpresponse writes shared HTTP response formats used across transport layers.
package response

import (
	"net/http"

	httpkitresponse "github.com/containeroo/httpkit/response"
)

// FieldProblem describes a validation problem for one request field.
type FieldProblem struct {
	// Field stores the field value used by field problem.
	Field string
	// Message contains the message associated with field problem.
	Message string
}

// NewFieldProblem creates a validation problem for one request field.
func NewFieldProblem(field, message string) FieldProblem {
	return FieldProblem{
		Field:   field,
		Message: message,
	}
}

// Respond writes a JSON response with the supplied status code.
func Respond(w http.ResponseWriter, status int, value any) {
	httpkitresponse.WriteJSON(w, status, value)
}

// XML writes an indented XML response with an XML declaration.
func XML(w http.ResponseWriter, status int, value any) error {
	return httpkitresponse.WriteXML(w, status, value)
}

// Problem writes a consistent JSON error response with optional field problems.
func Problem(w http.ResponseWriter, status int, message string, problems ...FieldProblem) {
	fields := make(map[string]string, len(problems))
	for _, problem := range problems {
		fields[problem.Field] = problem.Message
	}

	Respond(w, status, map[string]any{
		"error":    message,
		"problems": fields,
	})
}
