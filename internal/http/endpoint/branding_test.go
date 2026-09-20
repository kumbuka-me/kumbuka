package endpoint

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type brandLogoServiceStub struct {
	logo appsettings.BrandLogo
	err  error
}

func (s *brandLogoServiceStub) BrandLogo(context.Context) (appsettings.BrandLogo, error) {
	return s.logo, s.err
}

func (*brandLogoServiceStub) SaveBrandLogo(context.Context, string, []byte, int64) error {
	return nil
}

func (*brandLogoServiceStub) ClearBrandLogo(context.Context, int64) error {
	return nil
}

func TestBrandLogoServesCustomLogo(t *testing.T) {
	t.Parallel()

	settings := &brandLogoServiceStub{logo: appsettings.BrandLogo{
		ContentType: "image/png",
		Data:        []byte("custom-logo"),
	}}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/brand/logo", nil)

	BrandLogo(settings, fstest.MapFS{}, testViewsLogger()).ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "image/png", response.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", response.Header().Get("Cache-Control"))
	assert.Equal(t, "custom-logo", response.Body.String())
}

func TestBrandLogoFallsBackToFavicon(t *testing.T) {
	t.Parallel()

	settings := &brandLogoServiceStub{err: domain.ErrNotFound}
	assets := fstest.MapFS{
		"favicon.svg": &fstest.MapFile{Data: []byte(`<svg viewBox="0 0 10 10"></svg>`)},
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/brand/logo", nil)

	BrandLogo(settings, assets, testViewsLogger()).ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "image/svg+xml", response.Header().Get("Content-Type"))
	assert.Contains(t, response.Header().Get("Content-Security-Policy"), "default-src 'none'")
	assert.Equal(t, `<svg viewBox="0 0 10 10"></svg>`, response.Body.String())
}

func TestBrandLogoReturnsErrorWhenFallbackIsMissing(t *testing.T) {
	t.Parallel()

	settings := &brandLogoServiceStub{err: domain.ErrNotFound}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/brand/logo", nil)

	BrandLogo(settings, fstest.MapFS{}, testViewsLogger()).ServeHTTP(response, request)

	assert.Equal(t, http.StatusInternalServerError, response.Code)
	require.Contains(t, response.Body.String(), "The request could not be processed")
}
