package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adminAttachmentServiceStub struct {
	attachments []domain.Attachment
	deletedID   int64
}

func (s *adminAttachmentServiceStub) Attachments(context.Context) ([]domain.Attachment, error) {
	return s.attachments, nil
}

func (s *adminAttachmentServiceStub) DeleteAttachment(_ context.Context, id int64, _ domain.User) error {
	s.deletedID = id
	return nil
}

func TestAdminAttachmentsFiltersMetadata(t *testing.T) {
	t.Parallel()

	service := &adminAttachmentServiceStub{attachments: []domain.Attachment{
		{ID: 1, Filename: "runbook.pdf", ContentType: "application/pdf", Uploader: "Administrator", CreatedAt: time.Now()},
		{ID: 2, Filename: "notes.txt", ContentType: "text/plain; charset=utf-8", Uploader: "Editor", CreatedAt: time.Now()},
	}}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	browserContext := browserContextLoaderStub{load: func(*http.Request, *webview.Views, string) (webview.Layout, error) {
		return webview.Layout{User: domain.User{ID: 1, Role: domain.UserRoleAdmin}}, nil
	}}

	request := httptest.NewRequest(http.MethodGet, "/admin/attachments?attachment_q=runbook", nil)
	response := httptest.NewRecorder()
	AdminAttachments(browserContext, service, views).ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, response.Body.String(), "runbook.pdf")
	assert.NotContains(t, response.Body.String(), "notes.txt")
}

func TestDeleteAdminAttachmentRedirectsToInventory(t *testing.T) {
	t.Parallel()

	service := &adminAttachmentServiceStub{}
	views := testHandlerViews(t, webview.RuntimeInfo{})
	request := auth.WithUser(
		httptest.NewRequest(http.MethodPost, "/admin/attachments/42/delete", nil),
		domain.User{ID: 1, Role: domain.UserRoleAdmin},
	)
	request.SetPathValue("id", "42")
	response := httptest.NewRecorder()

	DeleteAdminAttachment(service, views).ServeHTTP(response, request)

	require.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, int64(42), service.deletedID)
	assert.Equal(t, "/admin/attachments", response.Header().Get("Location"))
}
