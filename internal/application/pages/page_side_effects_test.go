package pages

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// failingPageSideEffects provides test state for failing page side effects behavior.
type failingPageSideEffects struct {
	pageSaveRepositoryStub
	// auditCalls counts audit calls observed by the test double.
	auditCalls int
	// mentionCalls counts mention calls observed by the test double.
	mentionCalls int
	// watchCalls counts watch calls observed by the test double.
	watchCalls int
}

func (s *failingPageSideEffects) LogAudit(context.Context, int64, string, string, string, string) error {
	s.auditCalls++
	return errors.New("audit database unavailable")
}
func (s *failingPageSideEffects) NotifyMentions(context.Context, int64, string, string, string) error {
	s.mentionCalls++
	return errors.New("notification database unavailable")
}
func (s *failingPageSideEffects) NotifyPageWatchers(context.Context, int64, string, string, string, string) error {
	s.watchCalls++
	return errors.New("watch notification database unavailable")
}
func TestPageSaveReportsSecondaryFailuresWithoutFailingMutation(t *testing.T) {
	var logs bytes.Buffer
	repository := &failingPageSideEffects{}
	mutations := NewMutations(repository, nil, repository, slog.New(slog.NewJSONHandler(&logs, nil)))
	page, err := mutations.Save(context.Background(), PageSaveInput{Slug: "example", Title: "Example", Markdown: "private page content", Status: "verified", Actor: domain.User{ID: 42}})
	require.NoError(t, err)
	assert.Equal(t, "example", page.Slug)
	assert.Equal(t, 1, repository.auditCalls)
	assert.Equal(t, 1, repository.mentionCalls)
	assert.Equal(t, 1, repository.watchCalls)
	assert.Contains(t, logs.String(), "audit database unavailable")
	assert.Contains(t, logs.String(), "notification database unavailable")
	assert.Contains(t, logs.String(), "watch notification database unavailable")
	assert.Contains(t, logs.String(), `"object_key":"example"`)
	assert.Contains(t, logs.String(), `"actor_id":42`)
	assert.NotContains(t, logs.String(), "private page content")
}

type mentionNotificationSenderStub struct {
	calls       int
	actorID     int64
	text        string
	title       string
	destination string
}

func (s *mentionNotificationSenderStub) SendMentions(_ context.Context, actorID int64, text, title, destination string) error {
	s.calls++
	s.actorID = actorID
	s.text = text
	s.title = title
	s.destination = destination
	return nil
}

func TestPageSaveUsesSharedMentionNotificationSender(t *testing.T) {
	t.Parallel()

	repository := &failingPageSideEffects{}
	sender := &mentionNotificationSenderStub{}
	mutations := NewMutations(repository, nil, repository, slog.Default()).WithMentionNotifications(sender)

	page, err := mutations.Save(context.Background(), PageSaveInput{
		Slug:     "example",
		Title:    "Example",
		Markdown: "Please review, @admin",
		Status:   "verified",
		Actor:    domain.User{ID: 42},
	})

	require.NoError(t, err)
	assert.Equal(t, "example", page.Slug)
	assert.Equal(t, 1, sender.calls)
	assert.Equal(t, int64(42), sender.actorID)
	assert.Equal(t, "Please review, @admin", sender.text)
	assert.Equal(t, "Mention in Example", sender.title)
	assert.Equal(t, "/pages/example", sender.destination)
	assert.Zero(t, repository.mentionCalls)
}
