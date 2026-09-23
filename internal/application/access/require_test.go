package access

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiredPolicyStub provides controllable required policy behavior for tests.
type requiredPolicyStub struct {
	// view provides the callback invoked by the test double.
	view func(context.Context, domain.User, string) (bool, error)
	// edit provides the callback invoked by the test double.
	edit func(context.Context, domain.User, string) (bool, error)
}

func (p requiredPolicyStub) CanView(ctx context.Context, actor domain.User, path string) (bool, error) {
	return p.view(ctx, actor, path)
}
func (p requiredPolicyStub) CanEdit(ctx context.Context, actor domain.User, path string) (bool, error) {
	return p.edit(ctx, actor, path)
}

func TestRequireViewBlankPathSkipsPolicy(t *testing.T) {
	t.Parallel()
	require.NoError(t, RequireView(context.Background(), nil, domain.User{}, " \t\n"))
	require.NoError(t, RequireView(context.Background(), nil, domain.User{}, ""))
}
func TestRequireEditBlankPathSkipsPolicy(t *testing.T) {
	t.Parallel()
	require.NoError(t, RequireEdit(context.Background(), nil, domain.User{}, " \t\n"))
	require.NoError(t, RequireEdit(context.Background(), nil, domain.User{}, ""))
}
func TestRequireViewPassesRequestToPolicy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor := domain.User{ID: 42, Role: "viewer"}
	calls := 0
	policy := requiredPolicyStub{view: func(got context.Context, user domain.User, path string) (bool, error) {
		calls++
		assert.Same(t, ctx, got)
		assert.Equal(t, actor, user)
		assert.Equal(t, "/Private/Page/", path)
		return true, nil
	}}
	require.NoError(t, RequireView(ctx, policy, actor, "/Private/Page/"))
	assert.Equal(t, 1, calls)
}
func TestRequireEditPassesRequestToPolicy(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	actor := domain.User{ID: 42, Role: "editor"}
	calls := 0
	policy := requiredPolicyStub{edit: func(got context.Context, user domain.User, path string) (bool, error) {
		calls++
		assert.Same(t, ctx, got)
		assert.Equal(t, actor, user)
		assert.Equal(t, "/Private/Page/", path)
		return true, nil
	}}
	require.NoError(t, RequireEdit(ctx, policy, actor, "/Private/Page/"))
	assert.Equal(t, 1, calls)
}
func TestRequireViewHidesDeniedPage(t *testing.T) {
	t.Parallel()
	policy := requiredPolicyStub{view: func(context.Context, domain.User, string) (bool, error) { return false, nil }}
	require.ErrorIs(t, RequireView(context.Background(), policy, domain.User{}, "private"), domain.ErrNotFound)
}
func TestRequireEditRejectsDeniedPage(t *testing.T) {
	t.Parallel()
	policy := requiredPolicyStub{edit: func(context.Context, domain.User, string) (bool, error) { return false, nil }}
	require.ErrorIs(t, RequireEdit(context.Background(), policy, domain.User{}, "private"), domain.ErrForbidden)
}
func TestRequireViewPropagatesPolicyFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("policy unavailable")
	policy := requiredPolicyStub{view: func(context.Context, domain.User, string) (bool, error) { return true, failure }}
	require.ErrorIs(t, RequireView(context.Background(), policy, domain.User{}, "private"), failure)
}
func TestRequireEditPropagatesPolicyFailure(t *testing.T) {
	t.Parallel()
	failure := errors.New("policy unavailable")
	policy := requiredPolicyStub{edit: func(context.Context, domain.User, string) (bool, error) { return true, failure }}
	require.ErrorIs(t, RequireEdit(context.Background(), policy, domain.User{}, "private"), failure)
}
