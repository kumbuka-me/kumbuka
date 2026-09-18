package plugin

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// recordingAdminAction records invocation and optional failure for manager tests.
type recordingAdminAction struct {
	called bool
	err    error
}

// Run records one administrator invocation.
func (a *recordingAdminAction) Run(ctx context.Context) error {
	a.called = ctx != nil
	return a.err
}

// TestAdminActionUsesActiveContribution verifies explicit administrator actions run only through the active registry entry.
func TestAdminActionUsesActiveContribution(t *testing.T) {
	registry := &Registry{}
	action := &recordingAdminAction{}
	require.NoError(t, registry.Register(Descriptor{ID: "example.admin", Name: "Admin"}, Contributions{
		AdminActions: []AdminActionModule{{ID: "refresh", Name: "Refresh", Action: action}},
	}))

	manager := &Manager{registry: registry}
	require.NoError(t, manager.RunAdminAction(context.Background(), "example.admin", "refresh"))
	require.True(t, action.called)
}

// TestAdminActionRejectsMissingOrFailedActions verifies action errors stay scoped to the owning plugin.
func TestAdminActionRejectsMissingOrFailedActions(t *testing.T) {
	registry := &Registry{}
	action := &recordingAdminAction{err: errors.New("refresh failed")}
	require.NoError(t, registry.Register(Descriptor{ID: "example.admin", Name: "Admin"}, Contributions{
		AdminActions: []AdminActionModule{{ID: "refresh", Name: "Refresh", Action: action}},
	}))

	manager := &Manager{registry: registry}
	require.ErrorContains(t, manager.RunAdminAction(context.Background(), "example.admin", "missing"), "not active")
	require.ErrorContains(t, manager.RunAdminAction(context.Background(), "example.admin", "refresh"), "refresh failed")
	require.Error(t, manager.RunAdminAction(context.Background(), "missing.plugin", "refresh"))
}
