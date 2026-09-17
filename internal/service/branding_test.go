package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type brandLogoRepositoryStub struct {
	settingsRepository
	contentType string
	data        []byte
	cleared     bool
	action      string
}

func (s *brandLogoRepositoryStub) BrandLogo(context.Context) (string, []byte, error) {
	if len(s.data) == 0 {
		return "", nil, domain.ErrNotFound
	}

	return s.contentType, append([]byte(nil), s.data...), nil
}

func (s *brandLogoRepositoryStub) SaveBrandLogo(_ context.Context, contentType string, data []byte) error {
	s.contentType = contentType
	s.data = append([]byte(nil), data...)
	return nil
}

func (s *brandLogoRepositoryStub) ClearBrandLogo(context.Context) error {
	s.cleared = true
	s.contentType = ""
	s.data = nil
	return nil
}

func (s *brandLogoRepositoryStub) LogAudit(_ context.Context, _ int64, action, _, _, _ string) error {
	s.action = action
	return nil
}

func TestBrandLogoRoundTrip(t *testing.T) {
	t.Parallel()

	repository := &brandLogoRepositoryStub{}
	settings := NewSettings(repository, nil)
	data := []byte("\x89PNG\r\n\x1a\nlogo")

	err := settings.SaveBrandLogo(context.Background(), "logo.png", data, 7)
	require.NoError(t, err)
	assert.Equal(t, "image/png", repository.contentType)
	assert.Equal(t, "settings.brand_logo_updated", repository.action)

	logo, err := settings.BrandLogo(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "image/png", logo.ContentType)
	assert.Equal(t, data, logo.Data)
}

func TestSaveBrandLogoAcceptsPassiveSVG(t *testing.T) {
	t.Parallel()

	repository := &brandLogoRepositoryStub{}
	settings := NewSettings(repository, nil)
	data := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 10 10"><path d="M0 0h10v10H0z" /></svg>`)

	err := settings.SaveBrandLogo(context.Background(), "logo.svg", data, 7)

	require.NoError(t, err)
	assert.Equal(t, "image/svg+xml", repository.contentType)
	assert.Equal(t, data, repository.data)
}

func TestSaveBrandLogoRejectsActiveSVG(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "script", data: `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`},
		{name: "event handler", data: `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"></svg>`},
		{name: "external reference", data: `<svg xmlns="http://www.w3.org/2000/svg"><image href="https://example.test/logo.png" /></svg>`},
		{name: "doctype", data: `<!DOCTYPE svg><svg xmlns="http://www.w3.org/2000/svg"></svg>`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			settings := NewSettings(&brandLogoRepositoryStub{}, nil)
			err := settings.SaveBrandLogo(context.Background(), "logo.svg", []byte(test.data), 7)

			validation, ok := errors.AsType[*domain.ValidationError](err)
			require.True(t, ok)
			assert.Equal(t, "logo", validation.Fields[0].Field)
		})
	}
}

func TestSaveBrandLogoRejectsUnsupportedContent(t *testing.T) {
	t.Parallel()

	settings := NewSettings(&brandLogoRepositoryStub{}, nil)
	err := settings.SaveBrandLogo(context.Background(), "logo.txt", []byte("not an image"), 7)

	validation, ok := errors.AsType[*domain.ValidationError](err)
	require.True(t, ok)
	assert.Equal(t, "Choose a PNG, JPEG, GIF, WebP, or SVG image.", validation.UserMessage())
}

func TestClearBrandLogoRestoresDefault(t *testing.T) {
	t.Parallel()

	repository := &brandLogoRepositoryStub{contentType: "image/png", data: []byte("logo")}
	settings := NewSettings(repository, nil)

	err := settings.ClearBrandLogo(context.Background(), 7)

	require.NoError(t, err)
	assert.True(t, repository.cleared)
	assert.Equal(t, "settings.brand_logo_reset", repository.action)
}
