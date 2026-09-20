package endpoint

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type robotsSettingsStub struct {
	settings domain.ApplicationSettings
	err      error
}

func (s robotsSettingsStub) ApplicationSettings(context.Context) (domain.ApplicationSettings, error) {
	return s.settings, s.err
}

type sitemapCatalogStub struct {
	pages []domain.Page
	err   error
}

func (s sitemapCatalogStub) PageInventory(context.Context) ([]domain.Page, error) {
	return s.pages, s.err
}

func (s sitemapCatalogStub) PageInventoryFor(context.Context, domain.User) ([]domain.Page, error) {
	return s.pages, s.err
}

func TestRobots(t *testing.T) {
	t.Parallel()

	t.Run("allows indexing and advertises sitemap", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		handler := Robots(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyAllow}},
			testHandlerViews(t, webview.RuntimeInfo{PublicURL: "https://kumbuka.example.test/docs/"}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "text/plain; charset=utf-8", response.Header().Get("Content-Type"))
		assert.Equal(
			t,
			"User-agent: *\nAllow: /\nSitemap: https://kumbuka.example.test/docs/sitemap.xml\n",
			response.Body.String(),
		)
	})

	t.Run("disallows indexing", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		handler := Robots(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyDisallow}},
			testHandlerViews(t, webview.RuntimeInfo{}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "User-agent: *\nDisallow: /\n", response.Body.String())
	})

	t.Run("can be disabled", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		handler := Robots(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyNone}},
			testHandlerViews(t, webview.RuntimeInfo{}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("reports settings failures", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
		handler := Robots(robotsSettingsStub{err: errors.New("database unavailable")}, testHandlerViews(t, webview.RuntimeInfo{}), logger)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "database unavailable")
	})
}

func TestSitemap(t *testing.T) {
	t.Parallel()

	t.Run("includes only indexable pages", func(t *testing.T) {
		t.Parallel()

		updatedAt := time.Date(2026, time.September, 9, 14, 30, 0, 0, time.FixedZone("CEST", 2*60*60))
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		handler := Sitemap(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyAllow}},
			sitemapCatalogStub{pages: []domain.Page{
				{Slug: "archived", Status: "archived", UpdatedAt: updatedAt},
				{Slug: "draft", Status: "draft", UpdatedAt: updatedAt},
				{Slug: "deprecated-guide", Status: "deprecated", UpdatedAt: updatedAt},
				{Slug: "platform/start", Status: "verified", UpdatedAt: updatedAt},
			}},
			testHandlerViews(t, webview.RuntimeInfo{PublicURL: "https://kumbuka.example.test/docs/"}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/xml; charset=utf-8", response.Header().Get("Content-Type"))

		var document sitemapDocument
		require.NoError(t, xml.Unmarshal(response.Body.Bytes(), &document))
		assert.Equal(t, sitemapNamespace, document.XMLName.Space)
		assert.Equal(t, "urlset", document.XMLName.Local)
		assert.Equal(t, []sitemapEntry{
			{Location: "https://kumbuka.example.test/docs/"},
			{Location: "https://kumbuka.example.test/docs/pages/deprecated-guide", LastModified: "2026-09-09T12:30:00Z"},
			{Location: "https://kumbuka.example.test/docs/pages/platform/start", LastModified: "2026-09-09T12:30:00Z"},
		}, document.URLs)
		assert.NotContains(t, response.Body.String(), "/pages/draft")
		assert.NotContains(t, response.Body.String(), "/pages/archived")
	})

	t.Run("is hidden when crawling is disallowed", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		handler := Sitemap(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyDisallow}},
			sitemapCatalogStub{},
			testHandlerViews(t, webview.RuntimeInfo{}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("is hidden when crawler files are disabled", func(t *testing.T) {
		t.Parallel()

		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		handler := Sitemap(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyNone}},
			sitemapCatalogStub{},
			testHandlerViews(t, webview.RuntimeInfo{}),
			slog.Default(),
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusNotFound, response.Code)
	})

	t.Run("reports catalog failures", func(t *testing.T) {
		t.Parallel()

		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, nil))
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
		handler := Sitemap(
			robotsSettingsStub{settings: domain.ApplicationSettings{RobotsPolicy: domain.RobotsPolicyAllow}},
			sitemapCatalogStub{err: errors.New("inventory unavailable")},
			testHandlerViews(t, webview.RuntimeInfo{PublicURL: "https://kumbuka.example.test"}),
			logger,
		)

		handler.ServeHTTP(response, request)

		assert.Equal(t, http.StatusInternalServerError, response.Code)
		assert.Contains(t, logs.String(), "inventory unavailable")
	})
}
