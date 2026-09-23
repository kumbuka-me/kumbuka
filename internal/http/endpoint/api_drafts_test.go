package endpoint

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// draftContractService provides test state for draft contract service behavior.
type draftContractService struct {
	editorDraftService
	// draft configures or records the draft value used by the fixture.
	draft domain.PageDraft
}

func (s draftContractService) Draft(context.Context, int64, string) (domain.PageDraft, error) {
	return s.draft, nil
}
func (s draftContractService) Save(context.Context, apppages.PageDraftSaveInput) (domain.PageDraft, error) {
	return s.draft, nil
}

func TestDraftResponseContract(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../../../test/contracts/draft.json")
	require.NoError(t, err)
	at := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	drafts := draftContractService{draft: domain.PageDraft{ID: 1, Key: "new", CreatedAt: at, UpdatedAt: at}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Run("GET", func(t *testing.T) {
		t.Parallel()

		request := auth.WithUser(httptest.NewRequest("GET", "/api/drafts/new", strings.NewReader(`{"values":{}}`)), domain.User{ID: 7})
		request.SetPathValue("key", "new")
		response := httptest.NewRecorder()

		GetPageDraft(drafts, logger)(response, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(data), response.Body.String())
	})

	t.Run("PUT", func(t *testing.T) {
		t.Parallel()

		request := auth.WithUser(httptest.NewRequest("PUT", "/api/drafts/new", strings.NewReader(`{"values":{}}`)), domain.User{ID: 7})
		request.SetPathValue("key", "new")
		response := httptest.NewRecorder()

		SavePageDraft(drafts, logger)(response, request)

		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assert.JSONEq(t, string(data), response.Body.String())
	})
}

func TestDraftResponseNormalizesEmptySelectionsWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	draft := domain.PageDraft{Values: map[string][]string{"group_id": nil, "title": {"Example"}}}
	got := draftResponse(draft)

	assert.Equal(t, []string{}, got.Values["group_id"], "empty selection must be an array")
	assert.Nil(t, draft.Values["group_id"], "response mapping must not mutate input")
	assert.Equal(t, []string{"Example"}, got.Values["title"])
}
