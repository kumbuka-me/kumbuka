// Package httpresponse writes shared HTTP response formats used across transport layers.
package httpresponse

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
)

// FieldProblem describes a validation problem for one request field.
type FieldProblem struct {
	Field   string
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
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)

	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

// XML writes an indented XML response with an XML declaration.
func XML(w http.ResponseWriter, status int, value any) error {
	body, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}

	output := make([]byte, 0, len(xml.Header)+len(body)+1)
	output = append(output, xml.Header...)
	output = append(output, body...)
	output = append(output, '\n')

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(status)

	_, err = w.Write(output)
	return err
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
