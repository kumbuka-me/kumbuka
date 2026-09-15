package handler

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/httpresponse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupValidationProblems(t *testing.T) {
	t.Parallel()

	t.Run("accepts valid setup", func(t *testing.T) {
		t.Parallel()

		problems := setupProblems(t, url.Values{
			"username":         {"admin"},
			"email":            {"admin@example.com"},
			"password":         {"correct-horse-battery-staple"},
			"password_confirm": {"correct-horse-battery-staple"},
		})

		assert.Empty(t, problems)
	})

	t.Run("accepts empty optional email", func(t *testing.T) {
		t.Parallel()

		problems := setupProblems(t, url.Values{
			"username":         {"admin"},
			"password":         {"correct-horse-battery-staple"},
			"password_confirm": {"correct-horse-battery-staple"},
		})

		assert.Empty(t, problems)
	})

	t.Run("returns field problems", func(t *testing.T) {
		t.Parallel()

		problems := setupProblems(t, url.Values{
			"username":         {"   "},
			"email":            {"not-an-email"},
			"password":         {"short"},
			"password_confirm": {"different"},
		})

		assert.Equal(t, []httpresponse.FieldProblem{
			httpresponse.NewFieldProblem("username", "Username is required."),
			httpresponse.NewFieldProblem("email", "Enter a valid email address."),
			httpresponse.NewFieldProblem("password", "Use at least 12 characters."),
			httpresponse.NewFieldProblem("password_confirm", "Passwords do not match."),
		}, problems)
	})
}

func TestLocalPasswordValidationProblems(t *testing.T) {
	t.Parallel()

	t.Run("accepts matching password", func(t *testing.T) {
		t.Parallel()

		problems := localPasswordValidationProblems(
			"correct-horse-battery-staple",
			"correct-horse-battery-staple",
			"password",
			"password_confirm",
			true,
		)

		assert.Empty(t, problems)
	})

	t.Run("accepts empty optional password", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, localPasswordValidationProblems("", "", "password", "password_confirm", false))
	})

	t.Run("rejects empty required password", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []httpresponse.FieldProblem{
			httpresponse.NewFieldProblem("password", "Password is required."),
			httpresponse.NewFieldProblem("password_confirm", "Confirm the password."),
		}, localPasswordValidationProblems("", "", "password", "password_confirm", true))
	})

	t.Run("rejects short and mismatched password", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, []httpresponse.FieldProblem{
			httpresponse.NewFieldProblem("password", "Use at least 12 characters."),
			httpresponse.NewFieldProblem("password_confirm", "Passwords do not match."),
		}, localPasswordValidationProblems("short", "different", "password", "password_confirm", true))
	})
}

func TestValidEmailAddress(t *testing.T) {
	t.Parallel()

	assert.True(t, validEmailAddress("admin@example.com"))
	assert.False(t, validEmailAddress("not-an-email"))
	assert.False(t, validEmailAddress("Admin <admin@example.com>"))
}

func TestWantsJSON(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("POST", "/setup", nil)

	request.Header.Set("Accept", "text/html, application/json")
	assert.True(t, wantsJSON(request))
}

func setupProblems(t *testing.T, form url.Values) []httpresponse.FieldProblem {
	t.Helper()

	request := httptest.NewRequest("POST", "/setup", strings.NewReader(form.Encode()))

	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())
	return setupValidationProblems(request)
}

func TestLocalPasswordValidationRejectsBcryptOverflow(t *testing.T) {
	t.Parallel()
	password := strings.Repeat("a", 73)
	assert.Equal(t, []httpresponse.FieldProblem{
		httpresponse.NewFieldProblem("password", "Use at most 72 UTF-8 bytes."),
	}, localPasswordValidationProblems(password, password, "password", "password_confirm", true))
}
