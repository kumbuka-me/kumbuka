package handler

import (
	"net/http"
	"net/mail"
	"strings"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
)

// setupValidationProblems validates the fields required to create the initial administrator.
func setupValidationProblems(r *http.Request) []httpresponse.FieldProblem {
	var problems []httpresponse.FieldProblem

	if strings.TrimSpace(r.FormValue("username")) == "" {
		problems = append(problems, httpresponse.NewFieldProblem("username", "Username is required."))
	}
	if email := strings.TrimSpace(r.FormValue("email")); email != "" && !validEmailAddress(email) {
		problems = append(problems, httpresponse.NewFieldProblem("email", "Enter a valid email address."))
	}

	problems = append(problems, localPasswordValidationProblems(
		r.FormValue("password"),
		r.FormValue("password_confirm"),
		"password",
		"password_confirm",
		true,
	)...)

	return problems
}

// localPasswordValidationProblems validates a new local password and its confirmation.
// Optional password fields are ignored when both values are empty.
func localPasswordValidationProblems(
	password, confirmation, passwordField, confirmationField string,
	required bool,
) []httpresponse.FieldProblem {
	if !required && password == "" && confirmation == "" {
		return nil
	}

	var problems []httpresponse.FieldProblem

	if password == "" {
		problems = append(problems, httpresponse.NewFieldProblem(passwordField, "Password is required."))
	} else if problem := auth.LocalPasswordProblem(password); problem != "" {
		problems = append(problems, httpresponse.NewFieldProblem(passwordField, problem))
	}

	if confirmation == "" {
		problems = append(problems, httpresponse.NewFieldProblem(confirmationField, "Confirm the password."))
	} else if password != confirmation {
		problems = append(problems, httpresponse.NewFieldProblem(confirmationField, "Passwords do not match."))
	}

	return problems
}

// validEmailAddress reports whether value is one plain mailbox address.
func validEmailAddress(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value
}

// wantsJSON reports whether the caller explicitly requested a JSON response.
func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json")
}
