package plugincap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type directoryCapabilityStub struct{}

// SearchPublicUsers returns one safe fixture user.
func (directoryCapabilityStub) SearchPublicUsers(context.Context, string, int) ([]domain.User, error) {
	return []domain.User{{ID: 42, Username: "alice", Email: "private@example.test", DisplayName: "Alice", Enabled: true}}, nil
}

// UserByUsername returns one safe fixture user.
func (directoryCapabilityStub) UserByUsername(context.Context, string) (domain.User, error) {
	return domain.User{ID: 42, Username: "alice", Email: "private@example.test", DisplayName: "Alice", Enabled: true}, nil
}

type notificationCapabilityStub struct {
	actorID    int64
	sourceID   string
	sourceName string
}

// SendPlugin records host-supplied attribution.
func (s *notificationCapabilityStub) SendPlugin(_ context.Context, actorID int64, sourceID, sourceName string, input sdk.NotificationInput) (sdk.Notification, error) {
	s.actorID, s.sourceID, s.sourceName = actorID, sourceID, sourceName
	return sdk.Notification{ID: 7, RecipientUserID: input.RecipientUserID, CreatedAt: time.Now()}, nil
}

// TestPublicUsersExposeOnlySafeIdentity verifies public capability projection.
func TestPublicUsersExposeOnlySafeIdentity(t *testing.T) {
	value, err := (PublicUsers{Source: directoryCapabilityStub{}}).Search(
		context.Background(),
		sdk.UserQuery{Query: "ali", Limit: 10},
	)
	require.NoError(t, err)
	require.Len(t, value, 1)
	assert.Equal(t, sdk.User{ID: 42, Mention: "@alice", DisplayName: "Alice"}, value[0])
}

// TestNotificationCapabilitiesInjectTrustedAttribution verifies plugins cannot choose their source.
func TestNotificationCapabilitiesInjectTrustedAttribution(t *testing.T) {
	sender := &notificationCapabilityStub{}
	capability := NotificationCapabilities(sender, 9, "me.example.tasks", "Tasks")["notifications.send"]
	value, err := capability(context.Background(), json.RawMessage(`{"recipient_user_id":42,"title":"Assigned","idempotency_key":"one"}`))
	require.NoError(t, err)
	assert.Equal(t, int64(7), value.(sdk.Notification).ID)
	assert.Equal(t, int64(9), sender.actorID)
	assert.Equal(t, "me.example.tasks", sender.sourceID)
	assert.Equal(t, "Tasks", sender.sourceName)
}
