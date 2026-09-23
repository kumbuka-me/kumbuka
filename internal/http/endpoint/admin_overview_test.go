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

// adminOverviewStub provides controllable admin overview behavior for tests.
type adminOverviewStub struct {
	// stats configures or records the stats value used by the fixture.
	stats domain.AdminStats
	// attachments configures or records the attachments value used by the fixture.
	attachments int64
}

func (s adminOverviewStub) Stats(context.Context) (domain.AdminStats, error) {
	return s.stats, nil
}

func (s adminOverviewStub) AttachmentCount(context.Context) (int64, error) {
	return s.attachments, nil
}

// adminDatabaseInfoStub provides controllable admin database info behavior for tests.
type adminDatabaseInfoStub struct {
	// size configures or records the size value used by the fixture.
	size int64
}

func (s adminDatabaseInfoStub) DatabaseSize(context.Context) (int64, error) {
	return s.size, nil
}

func TestAdministrationOverviewShowsInstanceRuntimeAndInventory(t *testing.T) {
	t.Parallel()

	views := testHandlerViews(t, webview.RuntimeInfo{
		ListenAddress:             "127.0.0.1:51114",
		PublicURL:                 "http://localhost:8080",
		ReadOnly:                  true,
		PluginUpdateCheckInterval: "15m",
		EncryptionKeyConfigured:   true,
	})
	browserContext := browserContextLoaderStub{load: func(_ *http.Request, _ *webview.Views, title string) (webview.Layout, error) {
		return webview.Layout{
			Title:   title,
			Version: "v0.8.0",
			Commit:  "abc1234",
			Runtime: webview.RuntimeInfo{
				ListenAddress:             "127.0.0.1:51114",
				PublicURL:                 "http://localhost:8080",
				ReadOnly:                  true,
				PluginUpdateCheckInterval: "15m",
				EncryptionKeyConfigured:   true,
			},
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
	body := response.Body.String()
	assert.Contains(t, body, ">23</strong><span>Attachments</span>")
	assert.Contains(t, body, "Database size")
	assert.Contains(t, body, "42.0 MiB")
	assert.Contains(t, body, "v0.8.0")
	assert.Contains(t, body, "abc1234")
	assert.Contains(t, body, "127.0.0.1:51114")
	assert.Contains(t, body, "http://localhost:8080")
	assert.Contains(t, body, "Read-only mode")
	assert.Contains(t, body, "Enabled")
	assert.Contains(t, body, "Plugin update checks")
	assert.Contains(t, body, "15m")
	assert.Contains(t, body, "Application encryption")
	assert.Contains(t, body, "Configured")
	assert.Contains(t, body, `href="/admin/configuration#managed"`)
	assert.Contains(t, body, "Managed configuration")
}
