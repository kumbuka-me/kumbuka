package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pagePresenceStub struct {
	editors []domain.PageEditorPresence
	slug    string
	userID  int64
	left    bool
}

func (s *pagePresenceStub) PageEditors(_ context.Context, slug string, userID int64) ([]domain.PageEditorPresence, error) {
	s.slug = slug
	s.userID = userID
	return s.editors, nil
}

func (s *pagePresenceStub) TouchPageEditor(_ context.Context, slug string, user domain.User) error {
	s.slug = slug
	s.userID = user.ID
	return nil
}

func (s *pagePresenceStub) LeavePageEditor(_ context.Context, slug string, user domain.User) error {
	s.slug = slug
	s.userID = user.ID
	s.left = true
	return nil
}

func TestPageEditorsExcludesCurrentUserThroughServiceContract(t *testing.T) {
	t.Parallel()
	presence := &pagePresenceStub{editors: []domain.PageEditorPresence{{UserID: 8, Name: "Anna", UpdatedAt: time.Unix(10, 0)}}}
	request := auth.WithUser(httptest.NewRequest(http.MethodGet, "/api/page-presence/guide", nil), domain.User{ID: 7})
	request.SetPathValue("slug", "guide")
	response := httptest.NewRecorder()

	PageEditors(presence, slog.New(slog.NewTextHandler(io.Discard, nil)))(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "guide", presence.slug)
	assert.Equal(t, int64(7), presence.userID)
	var payload struct {
		Editors []domain.PageEditorPresence `json:"editors"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &payload))
	require.Len(t, payload.Editors, 1)
	assert.Equal(t, "Anna", payload.Editors[0].Name)
}

func TestTouchAndLeavePageEditorUseAuthenticatedUser(t *testing.T) {
	t.Parallel()
	presence := &pagePresenceStub{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	user := domain.User{ID: 11}

	touch := auth.WithUser(httptest.NewRequest(http.MethodPut, "/api/page-presence/guide", nil), user)
	touch.SetPathValue("slug", "guide")
	touchResponse := httptest.NewRecorder()
	TouchPageEditor(presence, logger)(touchResponse, touch)
	require.Equal(t, http.StatusNoContent, touchResponse.Code)
	assert.Equal(t, int64(11), presence.userID)
	assert.Equal(t, "guide", presence.slug)

	leave := auth.WithUser(httptest.NewRequest(http.MethodDelete, "/api/page-presence/guide", nil), user)
	leave.SetPathValue("slug", "guide")
	leaveResponse := httptest.NewRecorder()
	LeavePageEditor(presence, logger)(leaveResponse, leave)
	require.Equal(t, http.StatusNoContent, leaveResponse.Code)
	assert.True(t, presence.left)
}
