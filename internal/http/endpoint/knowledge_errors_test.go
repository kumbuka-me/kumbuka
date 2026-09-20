package endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type moveErrorStub struct{ err error }

func (s moveErrorStub) Move(context.Context, string, string, domain.MovePageOptions, domain.User) error {
	return s.err
}

func TestMovePageFormValidationProblem(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	request := httptest.NewRequest(http.MethodPost, "/pages/source/move", strings.NewReader("slug="))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetPathValue("slug", "source")
	response := httptest.NewRecorder()
	err := fmt.Errorf("move: %w", &domain.ValidationError{Fields: []domain.FieldError{{
		Field: "slug", Message: "A destination path is required.",
	}}})

	MovePageForm(moveErrorStub{err: err}, logger)(response, request)

	assert.Equal(t, http.StatusUnprocessableEntity, response.Code)
	assert.JSONEq(t, `{"error":"Page validation failed.","problems":{"slug":"A destination path is required."}}`, response.Body.String())
	assert.Empty(t, logs.String())
}

type graphErrorStub struct{ err error }

func (s graphErrorStub) KnowledgeGraph(context.Context, int) (domain.KnowledgeGraph, error) {
	return domain.KnowledgeGraph{}, s.err
}

func (s graphErrorStub) KnowledgeGraphFor(context.Context, domain.User, int) (domain.KnowledgeGraph, error) {
	return domain.KnowledgeGraph{}, s.err
}

func TestKnowledgeGraphFailureIsUnexpected(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	request := httptest.NewRequest(http.MethodGet, "/api/graph", nil)
	response := httptest.NewRecorder()
	err := fmt.Errorf("graph dependency: %w", domain.ErrNotFound)

	KnowledgeGraphAPI(graphErrorStub{err: err}, logger)(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)

	var problem struct {
		Error string `json:"error"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &problem))

	const prefix = "The request could not be processed. Reference: "
	require.True(t, strings.HasPrefix(problem.Error, prefix))

	reference := strings.TrimPrefix(problem.Error, prefix)

	assert.Regexp(
		t,
		`^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
		reference,
	)

	assert.Contains(t, logs.String(), "error_reference="+reference)
	assert.Contains(t, logs.String(), err.Error())
}

type savedSearchErrorStub struct {
	err error
}

func (s savedSearchErrorStub) SaveSavedSearch(context.Context, int64, int64, string, string, bool) error {
	return s.err
}

func (s savedSearchErrorStub) DeleteSavedSearch(context.Context, int64, int64) error { return s.err }

type membershipErrorStub struct {
	groupWriter
	err error
}

func (s membershipErrorStub) AddGroupMember(context.Context, int64, int64) error    { return s.err }
func (s membershipErrorStub) RemoveGroupMember(context.Context, int64, int64) error { return s.err }

func TestKnownServiceErrorsReachHTTPTranslators(t *testing.T) {
	t.Parallel()
	missing := fmt.Errorf("repository: %w", domain.ErrNotFound)
	conflict := fmt.Errorf("repository: %w", domain.ErrAlreadyExists)

	t.Run("create search conflict", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("name=test&query=test"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetPathValue("id", "1")
		request.SetPathValue("userID", "1")
		request.SetPathValue("slug", "source")
		request = auth.WithUser(request, domain.User{ID: 1, Role: "admin"})
		response := httptest.NewRecorder()
		CreateSavedSearch(savedSearchErrorStub{err: conflict}, logger)(response, request)
		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), "Saved search already exists.")
		assert.Empty(t, logs.String())
	})

	t.Run("delete missing search", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		request.Header.Set("Content-Type", "")
		request.SetPathValue("id", "1")
		request.SetPathValue("userID", "1")
		request.SetPathValue("slug", "source")
		request = auth.WithUser(request, domain.User{ID: 1, Role: "admin"})
		response := httptest.NewRecorder()
		DeleteSavedSearch(savedSearchErrorStub{err: missing}, logger)(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Contains(t, response.Body.String(), "Saved search not found.")
		assert.Empty(t, logs.String())
	})

	t.Run("add missing membership", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"user_id":1}`))
		request.Header.Set("Content-Type", "application/json")
		request.SetPathValue("id", "1")
		request.SetPathValue("userID", "1")
		request.SetPathValue("slug", "source")
		request = auth.WithUser(request, domain.User{ID: 1, Role: "admin"})
		response := httptest.NewRecorder()
		AddAdminGroupMember(membershipErrorStub{err: missing}, nil, logger)(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Contains(t, response.Body.String(), "Group or user not found.")
		assert.Empty(t, logs.String())
	})

	t.Run("remove missing membership", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(""))
		request.Header.Set("Content-Type", "")
		request.SetPathValue("id", "1")
		request.SetPathValue("userID", "1")
		request.SetPathValue("slug", "source")
		request = auth.WithUser(request, domain.User{ID: 1, Role: "admin"})
		response := httptest.NewRecorder()
		RemoveAdminGroupMember(membershipErrorStub{err: missing}, logger)(response, request)
		assert.Equal(t, http.StatusNotFound, response.Code)
		assert.Contains(t, response.Body.String(), "Group membership not found.")
		assert.Empty(t, logs.String())
	})

	t.Run("move path conflict", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("slug=target"))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.SetPathValue("id", "1")
		request.SetPathValue("userID", "1")
		request.SetPathValue("slug", "source")
		request = auth.WithUser(request, domain.User{ID: 1, Role: "admin"})
		response := httptest.NewRecorder()
		MovePageForm(moveErrorStub{err: conflict}, logger)(response, request)
		assert.Equal(t, http.StatusConflict, response.Code)
		assert.Contains(t, response.Body.String(), "Page path already exists.")
		assert.Empty(t, logs.String())
	})
}

type aliasFailureStub struct {
	pageReportService
	err error
}

func (s aliasFailureStub) GetPage(context.Context, string) (domain.Page, error) {
	return domain.Page{}, domain.ErrNotFound
}

func (s aliasFailureStub) ResolvePageAlias(context.Context, string) (string, error) { return "", s.err }

func (aliasFailureStub) RecordView(context.Context, string, int64) error { return nil }

func (aliasFailureStub) IsFavorite(context.Context, string, int64) (bool, error) { return false, nil }

func (aliasFailureStub) PageWatch(context.Context, string, int64) (domain.PageWatch, error) {
	return domain.PageWatch{}, nil
}

func (aliasFailureStub) PageLinks(context.Context, string) ([]domain.PageLink, error) {
	return nil, nil
}

func (aliasFailureStub) PageComments(context.Context, string) ([]domain.PageComment, error) {
	return nil, nil
}

func (s aliasFailureStub) GetPageFor(context.Context, domain.User, string) (domain.Page, error) {
	return domain.Page{}, domain.ErrNotFound
}

func (s aliasFailureStub) GetPageOrAliasFor(context.Context, domain.User, string) (domain.Page, string, error) {
	return domain.Page{}, "", s.err
}

func TestAliasFailureIsNotDiscarded(t *testing.T) {
	t.Parallel()
	t.Run("page view", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		repository := aliasFailureStub{err: fmt.Errorf("alias database offline")}
		request := httptest.NewRequest(http.MethodGet, "/pages/missing", nil)
		request.SetPathValue("slug", "missing")
		response := httptest.NewRecorder()

		ViewPage(nil, repository, nil, apppages.NewView(repository, browserContextAccessStub{}, nil, logger), nil, testHandlerViewsWithLogger(t, logger, webview.RuntimeInfo{}))(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "alias database offline")
		assert.NotContains(t, response.Body.String(), "alias database offline")
	})

	t.Run("API", func(t *testing.T) {
		t.Parallel()
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		repository := aliasFailureStub{err: fmt.Errorf("alias database offline")}
		request := httptest.NewRequest(http.MethodGet, "/pages/missing", nil)
		request.SetPathValue("slug", "missing")
		response := httptest.NewRecorder()

		GetPage(repository, logger)(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "alias database offline")
		assert.NotContains(t, response.Body.String(), "alias database offline")
	})
}
