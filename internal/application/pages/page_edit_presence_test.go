package pages

import (
	"context"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pagePresenceRepositoryStub provides controllable page presence repository behavior for tests.
type pagePresenceRepositoryStub struct {
	// editors configures or records the editors value used by the fixture.
	editors []domain.PageEditorPresence
	// slug records the slug observed by the test double.
	slug string
	// excludeUserID records the exclude user ID observed by the test double.
	excludeUserID int64
	// activeWithin configures or records the active within value used by the fixture.
	activeWithin time.Duration
	// touchedUserID records the user ID passed to touch operations.
	touchedUserID int64
	// departedUserID records the user ID passed to leave operations.
	departedUserID int64
}

func (s *pagePresenceRepositoryStub) PageEditors(
	_ context.Context,
	slug string,
	excludeUserID int64,
	activeWithin time.Duration,
) ([]domain.PageEditorPresence, error) {
	s.slug = slug
	s.excludeUserID = excludeUserID
	s.activeWithin = activeWithin
	return s.editors, nil
}

func (s *pagePresenceRepositoryStub) TouchPageEditor(_ context.Context, slug string, userID int64) error {
	s.slug = slug
	s.touchedUserID = userID
	return nil
}

func (s *pagePresenceRepositoryStub) LeavePageEditor(_ context.Context, slug string, userID int64) error {
	s.slug = slug
	s.departedUserID = userID
	return nil
}

func TestPagesEditorPresenceUsesBoundedHeartbeatWindow(t *testing.T) {
	t.Parallel()
	repository := &pagePresenceRepositoryStub{editors: []domain.PageEditorPresence{{UserID: 8, Name: "Anna"}}}
	presence := NewPresence(repository, nil)

	editors, err := presence.PageEditors(context.Background(), " guide ", domain.User{ID: 7})

	require.NoError(t, err)
	require.Len(t, editors, 1)
	assert.Equal(t, "guide", repository.slug)
	assert.Equal(t, int64(7), repository.excludeUserID)
	assert.Equal(t, pageEditorPresenceTTL, repository.activeWithin)
}

func TestPagesEditorPresenceTracksAuthenticatedEditor(t *testing.T) {
	t.Parallel()
	repository := &pagePresenceRepositoryStub{}
	presence := NewPresence(repository, nil)
	actor := domain.User{ID: 9}

	require.NoError(t, presence.TouchPageEditor(context.Background(), "/guide/", actor))
	assert.Equal(t, "guide", repository.slug)
	assert.Equal(t, int64(9), repository.touchedUserID)

	require.NoError(t, presence.LeavePageEditor(context.Background(), "/guide/", actor))
	assert.Equal(t, int64(9), repository.departedUserID)
}
