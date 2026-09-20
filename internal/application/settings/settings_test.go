package settings

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/internal/secrets"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pdfSettingsRepositoryStub struct {
	settingsRepository
	headers      []domain.PDFHeader
	savedURL     string
	savedHeaders []domain.PDFHeader
}

func (s *pdfSettingsRepositoryStub) PDFHeaders(context.Context) ([]domain.PDFHeader, error) {
	return append([]domain.PDFHeader(nil), s.headers...), nil
}

func (s *pdfSettingsRepositoryStub) SavePDFSettings(
	_ context.Context,
	pdfURL string,
	headers []domain.PDFHeader,
) error {
	s.savedURL = pdfURL
	s.savedHeaders = append([]domain.PDFHeader(nil), headers...)
	return nil
}

func (*pdfSettingsRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

type applicationSettingsRepositoryStub struct {
	settingsRepository
	saved domain.ApplicationSettings
}

func (s *applicationSettingsRepositoryStub) SaveApplicationSettings(
	_ context.Context,
	settings domain.ApplicationSettings,
) error {
	s.saved = settings
	return nil
}

func (*applicationSettingsRepositoryStub) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

func TestSaveApplicationSettingsValidatesExternalLinks(t *testing.T) {
	t.Parallel()

	t.Run("normalizes links", func(t *testing.T) {
		t.Parallel()

		repository := &applicationSettingsRepositoryStub{}
		settings := NewSettings(repository, nil)

		err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
			Rendering:    domain.RenderingSettings{DefaultTypographySize: domain.TypographySizeCompact},
			RobotsPolicy: domain.RobotsPolicyDisallow,
			ExternalLinks: []domain.ExternalLink{{
				Label:       " Repository ",
				URL:         " https://github.com/kumbuka-me/kumbuka ",
				Icon:        " book-lucide ",
				Description: " v2.4.1 ",
			}},
		}, 7)

		require.NoError(t, err)
		assert.Equal(t, []domain.ExternalLink{{
			Label:       "Repository",
			URL:         "https://github.com/kumbuka-me/kumbuka",
			Icon:        "book-lucide",
			Description: "v2.4.1",
			HoverEffect: domain.ExternalLinkHoverHighlight,
		}}, repository.saved.ExternalLinks)
	})

	t.Run("rejects non HTTP URL", func(t *testing.T) {
		t.Parallel()

		settings := NewSettings(&applicationSettingsRepositoryStub{}, nil)

		err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
			Rendering:     domain.RenderingSettings{DefaultTypographySize: domain.TypographySizeCompact},
			RobotsPolicy:  domain.RobotsPolicyDisallow,
			ExternalLinks: []domain.ExternalLink{{Label: "Repository", URL: "javascript:alert(1)"}},
		}, 7)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "external_links", validation.Fields[0].Field)
		assert.Equal(t, "Enter a valid HTTP or HTTPS URL for every external link.", validation.UserMessage())
	})

	t.Run("rejects unknown icon", func(t *testing.T) {
		t.Parallel()

		settings := NewSettings(&applicationSettingsRepositoryStub{}, nil)

		err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
			Rendering:     domain.RenderingSettings{DefaultTypographySize: domain.TypographySizeCompact},
			RobotsPolicy:  domain.RobotsPolicyDisallow,
			ExternalLinks: []domain.ExternalLink{{Label: "Repository", URL: "https://example.test", Icon: "not-an-icon"}},
		}, 7)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "Choose external link icons from the available icon catalog.", validation.UserMessage())
	})

	t.Run("rejects unknown hover effect", func(t *testing.T) {
		t.Parallel()

		settings := NewSettings(&applicationSettingsRepositoryStub{}, nil)

		err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
			Rendering:     domain.RenderingSettings{DefaultTypographySize: domain.TypographySizeCompact},
			RobotsPolicy:  domain.RobotsPolicyDisallow,
			ExternalLinks: []domain.ExternalLink{{Label: "Repository", URL: "https://example.test", HoverEffect: "bounce"}},
		}, 7)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "Choose a valid hover effect for every external link.", validation.UserMessage())
	})
}

func TestSaveApplicationSettingsValidatesRobotsPolicy(t *testing.T) {
	t.Parallel()

	settings := NewSettings(&applicationSettingsRepositoryStub{}, nil)
	err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
		RobotsPolicy: "invalid",
		Rendering:    domain.RenderingSettings{DefaultTypographySize: domain.TypographySizeCompact},
	}, 7)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "robots_policy", validation.Fields[0].Field)
	assert.Equal(t, "Choose a valid robots.txt policy.", validation.UserMessage())
}

func TestSaveApplicationSettingsValidatesTypographySize(t *testing.T) {
	t.Parallel()

	settings := NewSettings(&applicationSettingsRepositoryStub{}, nil)
	err := settings.SaveApplicationSettings(context.Background(), domain.ApplicationSettings{
		Rendering: domain.RenderingSettings{DefaultTypographySize: "huge"},
	}, 7)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "default_typography_size", validation.Fields[0].Field)
	assert.Equal(t, "Choose a valid default typography size.", validation.UserMessage())
}

func TestPDFHeadersMaskSensitiveValues(t *testing.T) {
	t.Parallel()

	repository := &pdfSettingsRepositoryStub{headers: []domain.PDFHeader{
		{ID: 1, Name: "Authorization", Value: "v1:ciphertext", Sensitive: true},
		{ID: 2, Name: "X-Tenant", Value: "documentation"},
	}}
	settings := NewSettings(repository, nil)

	headers, err := settings.PDFHeaders(context.Background())

	require.NoError(t, err)
	require.Len(t, headers, 2)
	assert.Empty(t, headers[0].Value)
	assert.True(t, headers[0].Configured)
	assert.Equal(t, "documentation", headers[1].Value)
	assert.False(t, headers[1].Configured)
}

func TestSavePDFSettingsEncryptsSensitiveValues(t *testing.T) {
	t.Parallel()

	cipher := testSecretCipher(t)
	repository := &pdfSettingsRepositoryStub{}
	settings := NewSettings(repository, cipher)

	err := settings.SavePDFSettings(context.Background(), "http://pdf/render", []PDFHeaderInput{
		{Name: "authorization", Value: "Bearer secret-token", Sensitive: true},
		{Name: "X-Tenant", Value: " documentation "},
	}, 7)

	require.NoError(t, err)
	assert.Equal(t, "http://pdf/render", repository.savedURL)
	require.Len(t, repository.savedHeaders, 2)
	assert.Equal(t, "authorization", repository.savedHeaders[0].Name)
	assert.True(t, repository.savedHeaders[0].Sensitive)
	assert.NotEqual(t, "Bearer secret-token", repository.savedHeaders[0].Value)
	assert.NotContains(t, repository.savedHeaders[0].Value, "secret-token")
	assert.Equal(t, "X-Tenant", repository.savedHeaders[1].Name)
	assert.Equal(t, " documentation ", repository.savedHeaders[1].Value)

	value, err := cipher.Decrypt(repository.savedHeaders[0].Value)
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", value)
}

func TestResolvePDFRequestHeadersUsesStoredSensitiveValue(t *testing.T) {
	t.Parallel()

	cipher := testSecretCipher(t)
	encrypted, err := cipher.Encrypt("Bearer stored-token")
	require.NoError(t, err)

	repository := &pdfSettingsRepositoryStub{headers: []domain.PDFHeader{
		{ID: 12, Name: "Authorization", Value: encrypted, Sensitive: true},
	}}
	settings := NewSettings(repository, cipher)

	headers, err := settings.ResolvePDFRequestHeaders(context.Background(), []PDFHeaderInput{
		{ID: 12, Name: "Authorization", Sensitive: true},
	})

	require.NoError(t, err)
	assert.Equal(t, []domain.PDFHeader{{ID: 12, Name: "Authorization", Value: "Bearer stored-token", Sensitive: true}}, headers)
}

func TestSavePDFSettingsRequiresEncryptionKeyForSensitiveValues(t *testing.T) {
	t.Parallel()

	cipher, err := secrets.New("")
	require.NoError(t, err)
	settings := NewSettings(&pdfSettingsRepositoryStub{}, cipher)

	err = settings.SavePDFSettings(context.Background(), "http://pdf/render", []PDFHeaderInput{
		{Name: "Authorization", Value: "Bearer secret-token", Sensitive: true},
	}, 7)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "Configure KUMBUKA__ENCRYPTION_KEY before saving sensitive PDF headers.", validation.UserMessage())
}

func TestSavePDFSettingsRejectsUnsafeAndDuplicateHeaders(t *testing.T) {
	t.Parallel()

	settings := NewSettings(&pdfSettingsRepositoryStub{}, nil)

	t.Run("unsafe request header", func(t *testing.T) {
		err := settings.SavePDFSettings(context.Background(), "http://pdf/render", []PDFHeaderInput{
			{Name: "content-length", Value: "1"},
		}, 7)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Contains(t, validation.UserMessage(), "cannot be configured")
	})

	t.Run("duplicate header name", func(t *testing.T) {
		err := settings.SavePDFSettings(context.Background(), "http://pdf/render", []PDFHeaderInput{
			{Name: "X-Tenant", Value: "one"},
			{Name: "x-tenant", Value: "two"},
		}, 7)

		validation, ok := errors.AsType[*domain.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, "PDF header names must be unique.", validation.UserMessage())
	})
}

func TestRevealPDFHeaderDecryptsSensitiveValue(t *testing.T) {
	t.Parallel()

	cipher := testSecretCipher(t)
	encrypted, err := cipher.Encrypt("secret-token")
	require.NoError(t, err)

	settings := NewSettings(&pdfSettingsRepositoryStub{headers: []domain.PDFHeader{
		{ID: 9, Name: "X-API-Key", Value: encrypted, Sensitive: true},
	}}, cipher)

	value, err := settings.RevealPDFHeader(context.Background(), 9)

	require.NoError(t, err)
	assert.Equal(t, "secret-token", value)
}

func testSecretCipher(t *testing.T) *secrets.Cipher {
	t.Helper()

	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	cipher, err := secrets.New(key)
	require.NoError(t, err)

	return cipher
}
