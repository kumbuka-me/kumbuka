package endpoint

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageTemplateInputFromFormParsesOptionalNumbers(t *testing.T) {
	t.Parallel()

	form := url.Values{
		"name":                 {"Runbook"},
		"owner_group_id":       {" 42 "},
		"review_interval_days": {" 90 "},
	}
	request := httptest.NewRequest("POST", "/admin/templates", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	require.NoError(t, request.ParseForm())

	input, err := pageTemplateInputFromForm(request)

	require.NoError(t, err)
	assert.Equal(t, int64(42), input.OwnerGroupID)
	assert.Equal(t, 90, input.ReviewIntervalDays)
}

func TestPageTemplateInputFromFormRejectsMalformedNumbers(t *testing.T) {
	t.Parallel()

	t.Run("owner group", func(t *testing.T) {
		t.Parallel()

		form := url.Values{"owner_group_id": {"not-a-number"}}
		request := httptest.NewRequest("POST", "/admin/templates", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.NoError(t, request.ParseForm())

		_, err := pageTemplateInputFromForm(request)

		require.Error(t, err)
	})

	t.Run("review interval", func(t *testing.T) {
		t.Parallel()

		form := url.Values{"review_interval_days": {"not-a-number"}}
		request := httptest.NewRequest("POST", "/admin/templates", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.NoError(t, request.ParseForm())

		_, err := pageTemplateInputFromForm(request)

		require.Error(t, err)
	})
}
