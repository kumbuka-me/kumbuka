package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/stretchr/testify/assert"
)

// editorToolbarSettingsStub provides controllable editor toolbar settings behavior for tests.
type editorToolbarSettingsStub struct {
	// current configures or records the current value used by the fixture.
	current domain.ApplicationSettings
	// saved configures or records the saved value used by the fixture.
	saved []domain.EditorToolbarOverride
	// actorID records the actor ID observed by the test double.
	actorID int64
}

func (s *editorToolbarSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.current, nil
}

func (s *editorToolbarSettingsStub) SaveEditorToolbarOverrides(
	_ context.Context,
	overrides []domain.EditorToolbarOverride,
	actorID int64,
) error {
	s.saved = append([]domain.EditorToolbarOverride(nil), overrides...)
	s.actorID = actorID
	return nil
}

func TestSaveAdminEditorToolbarUsesDedicatedSettingsMutation(t *testing.T) {
	t.Parallel()

	stale := domain.EditorToolbarOverride{ID: "missing:contribution", Group: "plugins", Hidden: true, Order: 7}
	settings := &editorToolbarSettingsStub{current: domain.ApplicationSettings{
		EditorToolbarOverrides: []domain.EditorToolbarOverride{stale},
		Rendering:              domain.RenderingSettings{DefaultTypographySize: "invalid"},
		RobotsPolicy:           "invalid",
	}}
	manager := plugin.NewManager(&plugin.Registry{}, nil)
	form := url.Values{}
	request := httptest.NewRequest(http.MethodPost, "/admin/editor-toolbar", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request = auth.WithUser(request, domain.User{ID: 7, Role: "admin"})
	response := httptest.NewRecorder()

	SaveAdminEditorToolbar(settings, manager, &webview.Views{})(response, request)

	assert.Equal(t, http.StatusSeeOther, response.Code)
	assert.Equal(t, "/admin/editor-toolbar", response.Header().Get("Location"))
	assert.Equal(t, []domain.EditorToolbarOverride{stale}, settings.saved)
	assert.Equal(t, int64(7), settings.actorID)
}
