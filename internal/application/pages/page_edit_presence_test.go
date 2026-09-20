package pages

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pagePresenceRepositoryStub struct {
	pageRepository
	editors        []domain.PageEditorPresence
	slug           string
	excludeUserID  int64
	activeWithin   time.Duration
	touchedUserID  int64
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
	pages := NewPages(repository, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))

	editors, err := pages.PageEditors(context.Background(), " guide ", domain.User{ID: 7})

	require.NoError(t, err)
	require.Len(t, editors, 1)
	assert.Equal(t, "guide", repository.slug)
	assert.Equal(t, int64(7), repository.excludeUserID)
	assert.Equal(t, pageEditorPresenceTTL, repository.activeWithin)
}

func TestPagesEditorPresenceTracksAuthenticatedEditor(t *testing.T) {
	t.Parallel()
	repository := &pagePresenceRepositoryStub{}
	pages := NewPages(repository, nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	actor := domain.User{ID: 9}

	require.NoError(t, pages.TouchPageEditor(context.Background(), "/guide/", actor))
	assert.Equal(t, "guide", repository.slug)
	assert.Equal(t, int64(9), repository.touchedUserID)

	require.NoError(t, pages.LeavePageEditor(context.Background(), "/guide/", actor))
	assert.Equal(t, int64(9), repository.departedUserID)
}
