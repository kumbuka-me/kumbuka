package endpoint

import (
	"context"
	"errors"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"net/http/httptest"
	"testing"
)

type pagePolicyStub struct {
	allowed bool
	err     error
}

func (s pagePolicyStub) CanView(context.Context, domain.User, string) (bool, error) {
	return s.allowed, s.err
}
func (s pagePolicyStub) CanEdit(context.Context, domain.User, string) (bool, error) {
	return s.allowed, s.err
}

func TestPageAccessResponseMapping(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		edit   bool
		policy pagePolicyStub
		status int
	}{
		{"hidden read", false, pagePolicyStub{}, 404},
		{"forbidden write", true, pagePolicyStub{}, 403},
		{"lookup failure", false, pagePolicyStub{err: errors.New("database secret")}, 500},
		{"allowed", false, pagePolicyStub{allowed: true}, 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/api/pages/private", nil)
			request.SetPathValue("slug", "private")
			response := httptest.NewRecorder()
			allowed := authorizePageRequest(response, request, test.policy, test.edit)
			assert.Equal(t, test.status == 200, allowed)
			assert.Equal(t, test.status, response.Code)
			assert.NotContains(t, response.Body.String(), "database secret")
		})
	}
}
