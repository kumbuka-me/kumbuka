package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type adminOverviewStub struct {
	stats       domain.AdminStats
	attachments int64
}

func (s adminOverviewStub) Stats(context.Context) (domain.AdminStats, error) {
	return s.stats, nil
}

func (s adminOverviewStub) AttachmentCount(context.Context) (int64, error) {
	return s.attachments, nil
}

type adminDatabaseInfoStub struct {
	size int64
}

func (s adminDatabaseInfoStub) DatabaseSize(context.Context) (int64, error) {
	return s.size, nil
}

func TestAdministrationOverviewShowsStorageSummary(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{})
	browserContext := browserContextLoaderStub{load: func(_ *http.Request, _ *webview.Views, title string) (webview.Layout, error) {
		return webview.Layout{
			Title:       title,
			User:        domain.User{ID: 1, Role: "admin", DisplayName: "Admin"},
			Preferences: domain.DefaultUserPreferences(),
		}, nil
	}}
	overview := adminOverviewStub{
		stats: domain.AdminStats{
			Users:        1,
			Groups:       2,
			Pages:        150,
			DeletedPages: 3,
			Tags:         58,
			Images:       154,
			Tokens:       4,
		},
		attachments: 23,
	}
	database := adminDatabaseInfoStub{size: 42 * 1024 * 1024}
	request := httptest.NewRequest(http.MethodGet, "/admin", nil)
	response := httptest.NewRecorder()

	Administration(browserContext, overview, database, views)(response, request)

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), ">23</strong><span>Attachments</span>")
	assert.Contains(t, response.Body.String(), ">42.0 MiB</strong><span>Database size</span>")
}
