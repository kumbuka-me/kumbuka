package pluginruntime

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// contentChangeQueueStub records durable queue operations for runtime tests.
type contentChangeQueueStub struct {
	// enqueued contains the most recently persisted per-plugin events.
	enqueued []domain.PluginContentChange
	// claimed contains the next batch returned by ClaimPluginContentChanges.
	claimed []domain.PluginContentChange
	// completed records acknowledged event IDs.
	completed []int64
	// retried records retry event IDs.
	retried []int64
	// retryAt records the most recent retry deadline.
	retryAt time.Time
	// retryError records the most recent persisted delivery error.
	retryError string
}

// EnqueuePluginContentChanges records one atomic queue batch.
func (s *contentChangeQueueStub) EnqueuePluginContentChanges(_ context.Context, changes []domain.PluginContentChange) error {
	s.enqueued = append([]domain.PluginContentChange(nil), changes...)
	return nil
}

// ClaimPluginContentChanges returns the configured batch once.
func (s *contentChangeQueueStub) ClaimPluginContentChanges(context.Context, int, time.Duration) ([]domain.PluginContentChange, error) {
	claimed := s.claimed
	s.claimed = nil
	return claimed, nil
}

// CompletePluginContentChange records one successful acknowledgement.
func (s *contentChangeQueueStub) CompletePluginContentChange(_ context.Context, id int64) error {
	s.completed = append(s.completed, id)
	return nil
}

// RetryPluginContentChange records one deferred delivery.
func (s *contentChangeQueueStub) RetryPluginContentChange(_ context.Context, id int64, availableAt time.Time, lastError string) error {
	s.retried = append(s.retried, id)
	s.retryAt = availableAt
	s.retryError = lastError
	return nil
}

// runtimeContentChangeStub records plugin deliveries and can fail a configured number of times.
type runtimeContentChangeStub struct {
	// calls contains requests delivered to the plugin hook.
	calls []plugin.ContentChangeRequest
	// failures is the number of upcoming calls that should fail.
	failures int
}

// Changed records one content-change invocation.
func (s *runtimeContentChangeStub) Changed(_ plugin.Context, request plugin.ContentChangeRequest) error {
	s.calls = append(s.calls, request)
	if s.failures > 0 {
		s.failures--
		return errors.New("temporary failure")
	}
	return nil
}

// contentChangeManagerForTest returns a manager with one active content-change hook.
func contentChangeManagerForTest(t *testing.T, handler plugin.ContentChange) *plugin.Manager {
	t.Helper()
	registry := &plugin.Registry{}
	require.NoError(t, registry.Register(
		plugin.Descriptor{ID: "me.kumbuka.tasks", Name: "Tasks"},
		plugin.Contributions{ContentChanges: []plugin.ContentChangeModule{{ID: "assignments", Handler: handler}}},
	))
	return plugin.NewManager(registry, nil)
}

// TestContentChangedQueuesCommittedMutation verifies page requests persist one event for each active target.
func TestContentChangedQueuesCommittedMutation(t *testing.T) {
	t.Parallel()
	queue := &contentChangeQueueStub{}
	handler := &runtimeContentChangeStub{}
	service := NewContentChanges(queue, contentChangeManagerForTest(t, handler), nil, nil)

	err := service.ContentChanged(context.Background(), pages.PageContentChange{
		Page:             domain.Page{Slug: "guide", Title: "Guide", Markdown: "new"},
		PreviousMarkdown: "old",
		Markdown:         "new",
		Actor:            domain.User{ID: 7},
	})

	require.NoError(t, err)
	require.Len(t, queue.enqueued, 1)
	assert.Equal(t, "me.kumbuka.tasks", queue.enqueued[0].PluginID)
	assert.Equal(t, "guide", queue.enqueued[0].Page.Slug)
	assert.Equal(t, "old", queue.enqueued[0].PreviousMarkdown)
	assert.Equal(t, "new", queue.enqueued[0].Markdown)
	assert.Equal(t, int64(7), queue.enqueued[0].ActorID)
	assert.Empty(t, handler.calls)
}

// TestProcessPendingAcknowledgesSuccessfulDelivery verifies durable rows are removed only after plugin success.
func TestProcessPendingAcknowledgesSuccessfulDelivery(t *testing.T) {
	t.Parallel()
	queue := &contentChangeQueueStub{claimed: []domain.PluginContentChange{{
		ID:               11,
		PluginID:         "me.kumbuka.tasks",
		Page:             domain.Page{Slug: "guide", Title: "Guide"},
		PreviousMarkdown: "old",
		Markdown:         "new",
		ActorID:          7,
		Attempts:         1,
	}}}
	handler := &runtimeContentChangeStub{}
	service := NewContentChanges(queue, contentChangeManagerForTest(t, handler), nil, nil)

	service.processPending(context.Background())

	require.Len(t, handler.calls, 1)
	assert.Equal(t, sdk.Page{Slug: "guide", Title: "Guide"}, handler.calls[0].Page)
	assert.Equal(t, []int64{11}, queue.completed)
	assert.Empty(t, queue.retried)
}

// TestProcessPendingRetriesFailedDelivery verifies transient plugin failures remain durable for a later attempt.
func TestProcessPendingRetriesFailedDelivery(t *testing.T) {
	t.Parallel()
	queue := &contentChangeQueueStub{claimed: []domain.PluginContentChange{{
		ID:               12,
		PluginID:         "me.kumbuka.tasks",
		Page:             domain.Page{Slug: "guide", Title: "Guide"},
		PreviousMarkdown: "old",
		Markdown:         "new",
		ActorID:          7,
		Attempts:         3,
	}}}
	handler := &runtimeContentChangeStub{failures: 1}
	service := NewContentChanges(queue, contentChangeManagerForTest(t, handler), nil, nil)

	before := time.Now()
	service.processPending(context.Background())

	assert.Empty(t, queue.completed)
	assert.Equal(t, []int64{12}, queue.retried)
	assert.True(t, queue.retryAt.After(before))
	assert.Contains(t, queue.retryError, "temporary failure")
}

// TestProcessPendingAcknowledgesMissingPlugin verifies queued work does not retry forever after a plugin is removed.
func TestProcessPendingAcknowledgesMissingPlugin(t *testing.T) {
	t.Parallel()
	queue := &contentChangeQueueStub{claimed: []domain.PluginContentChange{{
		ID:       13,
		PluginID: "me.kumbuka.removed",
		Page:     domain.Page{Slug: "guide"},
		Markdown: "new",
		ActorID:  7,
		Attempts: 1,
	}}}
	service := NewContentChanges(queue, contentChangeManagerForTest(t, &runtimeContentChangeStub{}), nil, nil)

	service.processPending(context.Background())

	assert.Equal(t, []int64{13}, queue.completed)
	assert.Empty(t, queue.retried)
}

// TestContentChangeRetryDelayCapsExponentialBackoff verifies queue retries remain responsive and bounded.
func TestContentChangeRetryDelayCapsExponentialBackoff(t *testing.T) {
	t.Parallel()
	assert.Equal(t, time.Second, contentChangeRetryDelay(1))
	assert.Equal(t, 4*time.Second, contentChangeRetryDelay(3))
	assert.Equal(t, contentChangeMaxBackoff, contentChangeRetryDelay(100))
}
