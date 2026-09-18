package handler

import (
	"bytes"
	"html/template"
	"os"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotificationHeaderUnreadClass(t *testing.T) {
	t.Parallel()

	source, err := os.ReadFile("../../web/src/templates/header.gohtml")
	require.NoError(t, err)

	tmpl, err := template.New("header").Funcs(template.FuncMap{
		"icon":          func(string, int) template.HTML { return "" },
		"logo":          func() template.HTML { return "" },
		"timeago":       func(time.Time) string { return "now" },
		"externalhover": domain.ExternalLinkHoverTitle,
		"externalhovereffect": func(link domain.ExternalLink) string {
			return domain.EffectiveExternalLinkHoverEffect(link.HoverEffect)
		},
	}).Parse(string(source))
	require.NoError(t, err)

	data := webview.Data{
		ApplicationSettings: domain.ApplicationSettings{ExternalLinks: []domain.ExternalLink{{
			Label:       "Repository",
			URL:         "https://github.com/kumbuka-me/kumbuka",
			Icon:        "github-simple",
			Description: "v2.4.1",
			HoverEffect: "lift",
			HoverText:   "{{label }} | {{description}}",
		}}},
		Notifications: []domain.Notification{{
			ID:        7,
			Kind:      "mention",
			Title:     "Mention",
			CreatedAt: time.Now(),
		}},
		UnreadNotifications: 1,
	}

	var output bytes.Buffer
	require.NoError(t, tmpl.ExecuteTemplate(&output, "header", data))

	assert.Contains(t, output.String(), `class="notification-item unread"`)
	assert.Contains(t, output.String(), `href="https://github.com/kumbuka-me/kumbuka"`)
	assert.Contains(t, output.String(), ">Repository</strong>")
	assert.Contains(t, output.String(), ">v2.4.1</small>")
	assert.Contains(t, output.String(), `class="external-link hover-lift has-icon"`)
	assert.Contains(t, output.String(), `title="Repository | v2.4.1"`)
	assert.NotContains(t, output.String(), `class="notification-itemunread"`)
}
