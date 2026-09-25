package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentChangeHandlerStub records mutation requests and optional failures.
type contentChangeHandlerStub struct {
	// request is the most recent mutation observed by the handler.
	request ContentChangeRequest
	// calls counts committed-content invocations.
	calls int
	// capability records whether the mutation-only capability was present.
	capability bool
	// err is the configured handler failure.
	err error
}

// Changed records one committed-content invocation.
func (s *contentChangeHandlerStub) Changed(scope Context, request ContentChangeRequest) error {
	s.request = request
	s.calls++
	s.capability = scope.Capabilities["notifications.send"] != nil
	return s.err
}

// contentChangeScopeForTest returns a mutation scope containing the notification capability.
func contentChangeScopeForTest(Descriptor) Context {
	return Context{Capabilities: map[string]Capability{
		"notifications.send": func(context.Context, json.RawMessage) (any, error) { return nil, nil },
	}}
}

func TestContentChanged(t *testing.T) {
	t.Run("invokes all active hooks", func(t *testing.T) {
		t.Parallel()
		registry := &Registry{}
		first := &contentChangeHandlerStub{}
		second := &contentChangeHandlerStub{err: errors.New("failed")}
		require.NoError(t, registry.Register(
			Descriptor{ID: "example.first", Name: "First"},
			Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: first}}},
		))
		require.NoError(t, registry.Register(
			Descriptor{ID: "example.second", Name: "Second"},
			Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: second}}},
		))
		manager := NewManager(registry, nil)
		request := ContentChangeRequest{Page: sdk.Page{Slug: "guide"}, PreviousSource: "old", Source: "new"}

		err := manager.ContentChanged(context.Background(), request, contentChangeScopeForTest)

		require.Error(t, err)
		assert.Equal(t, request, first.request)
		assert.Equal(t, request, second.request)
		assert.True(t, first.capability)
		assert.Contains(t, err.Error(), "example.second")
	})

	t.Run("targets only one captured plugin", func(t *testing.T) {
		t.Parallel()
		registry := &Registry{}
		first := &contentChangeHandlerStub{}
		second := &contentChangeHandlerStub{}
		require.NoError(t, registry.Register(
			Descriptor{ID: "example.first", Name: "First"},
			Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: first}}},
		))
		require.NoError(t, registry.Register(
			Descriptor{ID: "example.second", Name: "Second"},
			Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: second}}},
		))
		manager := NewManager(registry, nil)
		request := ContentChangeRequest{Page: sdk.Page{Slug: "guide"}, PreviousSource: "old", Source: "new"}

		err := manager.ContentChangedFor(context.Background(), "example.first", request, contentChangeScopeForTest)

		require.NoError(t, err)
		assert.Equal(t, 1, first.calls)
		assert.Zero(t, second.calls)
	})

	t.Run("ignores a removed target", func(t *testing.T) {
		t.Parallel()
		registry := &Registry{}
		handler := &contentChangeHandlerStub{}
		require.NoError(t, registry.Register(
			Descriptor{ID: "example.first", Name: "First"},
			Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: handler}}},
		))
		manager := NewManager(registry, nil)
		require.NoError(t, registry.Unregister("example.first"))

		err := manager.ContentChangedFor(context.Background(), "example.first", ContentChangeRequest{}, contentChangeScopeForTest)

		require.NoError(t, err)
		assert.Zero(t, handler.calls)
	})
}

func TestContentChangeTargets(t *testing.T) {
	t.Parallel()
	registry := &Registry{}
	require.NoError(t, registry.Register(
		Descriptor{ID: "example.change", Name: "Change"},
		Contributions{ContentChanges: []ContentChangeModule{{ID: "change", Handler: &contentChangeHandlerStub{}}}},
	))
	require.NoError(t, registry.Register(
		Descriptor{ID: "example.render", Name: "Render"},
		Contributions{},
	))
	manager := NewManager(registry, nil)

	targets := manager.ContentChangeTargets()

	require.Len(t, targets, 1)
	assert.Equal(t, "example.change", targets[0].ID)
}
