package handler

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWebhookContext(t *testing.T) {
	t.Parallel()

	item := domain.Webhook{ID: 7, Name: "Deploy"}
	events := []string{"page.created", "page.updated"}

	view := webhookContext(item, events, true)

	assert.Equal(t, item, view.Webhook)
	assert.Equal(t, events, view.AvailableEvents)
	assert.True(t, view.EncryptionKeyConfigured)
}

func TestPageTemplateContext(t *testing.T) {
	t.Parallel()

	item := domain.PageTemplate{ID: 3, Name: "Service"}
	groups := []domain.Group{{ID: 9, Name: "Platform"}}
	statuses := []string{"draft", "verified"}

	view := pageTemplateContext(item, groups, statuses)

	assert.Equal(t, item, view.PageTemplate)
	assert.Equal(t, groups, view.Groups)
	assert.Equal(t, statuses, view.PageStatuses)
}

func TestBlankPageTemplate(t *testing.T) {
	t.Parallel()

	blueprint := blankPageTemplate()

	assert.Equal(t, "verified", blueprint.Status)
	assert.NotNil(t, blueprint.Properties)
	assert.Empty(t, blueprint.Properties)
}

func TestTemplatePropertiesText(t *testing.T) {
	t.Parallel()

	t.Run("sorts keys case insensitively", func(t *testing.T) {
		t.Parallel()

		properties := map[string]string{
			"Zone":        "eu-central-2",
			"environment": "production",
			"Owner":       "platform",
		}

		assert.Equal(
			t,
			"environment=production\nOwner=platform\nZone=eu-central-2",
			templatePropertiesText(properties),
		)
	})

	t.Run("returns empty text for no properties", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, templatePropertiesText(nil))
	})
}

func TestTemplateFieldsText(t *testing.T) {
	t.Parallel()

	t.Run("serializes optional and required fields", func(t *testing.T) {
		t.Parallel()

		fields := []domain.PageTemplateField{
			{Name: "service", Label: "Service", Default: "api"},
			{Name: "owner", Label: "Owner", Default: "platform", Required: true},
		}

		assert.Equal(
			t,
			"service | Service | api | \nowner | Owner | platform | required",
			templateFieldsText(fields),
		)
	})

	t.Run("returns empty text for no fields", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, templateFieldsText(nil))
	})
}

func TestTimeAgo(t *testing.T) {
	t.Parallel()

	t.Run("shows recent timestamps as just now", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "just now", timeAgo(time.Now().Add(-10*time.Second)))
	})

	t.Run("shows minutes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "5m ago", timeAgo(time.Now().Add(-5*time.Minute)))
	})

	t.Run("shows hours", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "2h ago", timeAgo(time.Now().Add(-2*time.Hour)))
	})

	t.Run("shows calendar date for older timestamps", func(t *testing.T) {
		t.Parallel()

		value := time.Now().Add(-48 * time.Hour)
		assert.Equal(t, value.Format("2006-01-02"), timeAgo(value))
	})
}

func TestFileSize(t *testing.T) {
	t.Parallel()

	t.Run("formats bytes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "512 B", fileSize(512))
	})

	t.Run("formats kibibytes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "1.5 KiB", fileSize(1536))
	})

	t.Run("formats mebibytes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "2.0 MiB", fileSize(2*1024*1024))
	})

	t.Run("formats gibibytes", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "3.0 GiB", fileSize(3*1024*1024*1024))
	})
}

// TestFingerprintAssets verifies embedded asset fingerprints are stable and content-sensitive.
func TestFingerprintAssets(t *testing.T) {
	t.Parallel()

	t.Run("is stable for identical assets", func(t *testing.T) {
		t.Parallel()

		assets := fstest.MapFS{
			"css/app.css": &fstest.MapFile{Data: []byte("body{}")},
			"js/main.js":  &fstest.MapFile{Data: []byte("console.log('one')")},
		}

		first, err := fingerprintAssets(assets)
		require.NoError(t, err)

		second, err := fingerprintAssets(assets)
		require.NoError(t, err)

		assert.Len(t, first, 16)
		assert.Equal(t, first, second)
	})

	t.Run("changes when asset content changes", func(t *testing.T) {
		t.Parallel()

		firstAssets := fstest.MapFS{
			"js/main.js": &fstest.MapFile{Data: []byte("console.log('one')")},
		}
		secondAssets := fstest.MapFS{
			"js/main.js": &fstest.MapFile{Data: []byte("console.log('two')")},
		}

		first, err := fingerprintAssets(firstAssets)
		require.NoError(t, err)

		second, err := fingerprintAssets(secondAssets)
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
	})

	t.Run("changes when asset path changes", func(t *testing.T) {
		t.Parallel()

		firstAssets := fstest.MapFS{
			"js/main.js": &fstest.MapFile{Data: []byte("same")},
		}
		secondAssets := fstest.MapFS{
			"js/app.js": &fstest.MapFile{Data: []byte("same")},
		}

		first, err := fingerprintAssets(firstAssets)
		require.NoError(t, err)

		second, err := fingerprintAssets(secondAssets)
		require.NoError(t, err)

		assert.NotEqual(t, first, second)
	})
}
