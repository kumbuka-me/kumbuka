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
	// pageSaveRepositoryStub is embedded to provide the default interface behavior for this fixture.
	pageSaveRepositoryStub
	// auditCalls counts audit calls observed by the test double.
	auditCalls int
	// watchCalls counts watcher-resolution calls observed by the test double.
	watchCalls int
}

func (s *failingPageSideEffects) LogAudit(context.Context, int64, string, string, string, string) error {
	s.auditCalls++
	return errors.New("audit database unavailable")
}

func (s *failingPageSideEffects) PageWatcherUserIDs(context.Context, int64, string) ([]int64, error) {
	s.watchCalls++
	return []int64{7}, nil
}

// failingNotificationSender records notification calls and returns delivery failures.
type failingNotificationSender struct {
	// mentionCalls counts mention notification deliveries.
	mentionCalls int
	// coreCalls counts core notification deliveries.
	coreCalls int
}

func (s *failingNotificationSender) SendMentions(context.Context, int64, string, string, string) error {
	s.mentionCalls++
	return errors.New("notification delivery unavailable")
}

func (s *failingNotificationSender) SendCore(context.Context, int64, int64, domain.NotificationKind, string, string, string) error {
	s.coreCalls++
	return errors.New("notification delivery unavailable")
}

func TestPageSaveReportsSecondaryFailuresWithoutFailingMutation(t *testing.T) {
	var logs bytes.Buffer
	repository := &failingPageSideEffects{}
	notifications := &failingNotificationSender{}
	mutations := NewMutations(repository, nil, repository, slog.New(slog.NewJSONHandler(&logs, nil))).WithNotifications(notifications)

	page, err := mutations.Save(context.Background(), PageSaveInput{Slug: "example", Title: "Example", Markdown: "private page content", Status: "verified", Actor: domain.User{ID: 42}})

	require.NoError(t, err)
	assert.Equal(t, "example", page.Slug)
	assert.Equal(t, 1, repository.auditCalls)
	assert.Equal(t, 1, repository.watchCalls)
	assert.Equal(t, 1, notifications.mentionCalls)
	assert.Equal(t, 1, notifications.coreCalls)
	assert.Contains(t, logs.String(), "audit database unavailable")
	assert.Contains(t, logs.String(), "notification delivery unavailable")
	assert.Contains(t, logs.String(), `"object_key":"example"`)
	assert.Contains(t, logs.String(), `"actor_id":42`)
	assert.NotContains(t, logs.String(), "private page content")
}

// notificationSenderStub captures page-side-effect notifications for assertions.
type notificationSenderStub struct {
	// calls counts calls made to the fixture.
	calls int
	// actorID captures the actor identifier supplied to the fixture.
	actorID int64
	// text captures the authored text supplied to the fixture.
	text string
	// title captures the title supplied to the fixture.
	title string
	// destination captures the destination URL supplied to the fixture.
	destination string
}

func (s *notificationSenderStub) SendMentions(_ context.Context, actorID int64, text, title, destination string) error {
	s.calls++
	s.actorID = actorID
	s.text = text
	s.title = title
	s.destination = destination
	return nil
}

func (*notificationSenderStub) SendCore(context.Context, int64, int64, domain.NotificationKind, string, string, string) error {
	return nil
}

func TestPageSaveUsesSharedMentionNotificationSender(t *testing.T) {
	t.Parallel()

	repository := &failingPageSideEffects{}
	sender := &notificationSenderStub{}
	mutations := NewMutations(repository, nil, repository, slog.Default()).WithNotifications(sender)

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
	assert.Equal(t, "/p/7/example", sender.destination)
}
