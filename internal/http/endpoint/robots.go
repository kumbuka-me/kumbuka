package endpoint

import (
	"context"
	"encoding/xml"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
)

const sitemapNamespace = "http://www.sitemaps.org/schemas/sitemap/0.9"

type robotsSettingsService interface {
	ApplicationSettings(context.Context) (domain.ApplicationSettings, error)
}

type sitemapCatalogService interface {
	PageInventory(context.Context) ([]domain.Page, error)
}

// sitemapDocument groups data used by sitemap document.
type sitemapDocument struct {
	// XMLName is the XML name associated with sitemap document.
	XMLName xml.Name `xml:"urlset"`
	// XMLNS stores the XMLNS value used by sitemap document.
	XMLNS string `xml:"xmlns,attr"`
	// URLs contains the UR ls associated with sitemap document.
	URLs []sitemapEntry `xml:"url"`
}

// sitemapEntry groups data used by sitemap entry.
type sitemapEntry struct {
	// Location stores the location value used by sitemap entry.
	Location string `xml:"loc"`
	// LastModified stores the last modified value used by sitemap entry.
	LastModified string `xml:"lastmod,omitempty"`
}

// Robots serves crawler guidance configured by an administrator.
func Robots(settingsUseCases robotsSettingsService, views *webview.Views, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		var body string

		switch settings.RobotsPolicy {
		case domain.RobotsPolicyAllow:
			sitemapURL, err := publicResourceURL(views.Runtime().PublicURL, "sitemap.xml")
			if err != nil {
				httpresponse.InternalServerError(logger, w, err)
				return
			}

			body = "User-agent: *\nAllow: /\nSitemap: " + sitemapURL + "\n"
		case domain.RobotsPolicyDisallow:
			body = "User-agent: *\nDisallow: /\n"
		case domain.RobotsPolicyNone:
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		default:
			httpresponse.InternalServerError(logger, w, errors.New("invalid persisted robots.txt policy"))
			return
		}

		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

// Sitemap serves the indexable application pages when crawling is enabled.
func Sitemap(
	settingsUseCases robotsSettingsService,
	catalogUseCases visiblePageInventoryService,
	views *webview.Views,
	logger *slog.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		settings, err := settingsUseCases.ApplicationSettings(r.Context())
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		switch settings.RobotsPolicy {
		case domain.RobotsPolicyAllow:
		case domain.RobotsPolicyDisallow, domain.RobotsPolicyNone:
			httpresponse.Problem(w, http.StatusNotFound, "Not found.")
			return
		default:
			httpresponse.InternalServerError(logger, w, errors.New("invalid persisted robots.txt policy"))
			return
		}

		pages, err := catalogUseCases.PageInventoryFor(r.Context(), domain.User{})
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		document, err := sitemapForPages(views.Runtime().PublicURL, pages)
		if err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}

		if err := httpresponse.XML(w, http.StatusOK, document); err != nil {
			httpresponse.InternalServerError(logger, w, err)
			return
		}
	}
}

// sitemapForPages builds the public sitemap from indexable application pages.
func sitemapForPages(publicURL string, pages []domain.Page) (sitemapDocument, error) {
	home, err := publicResourceURL(publicURL)
	if err != nil {
		return sitemapDocument{}, err
	}

	document := sitemapDocument{
		XMLNS: sitemapNamespace,
		URLs:  []sitemapEntry{{Location: home}},
	}

	for _, page := range pages {
		if !indexablePageStatus(page.Status) {
			continue
		}

		location, err := publicResourceURL(publicURL, "pages", page.Slug)
		if err != nil {
			return sitemapDocument{}, err
		}

		entry := sitemapEntry{Location: location}
		if !page.UpdatedAt.IsZero() {
			entry.LastModified = page.UpdatedAt.UTC().Format(time.RFC3339)
		}

		document.URLs = append(document.URLs, entry)
	}

	return document, nil
}

// indexablePageStatus reports whether a lifecycle state belongs in the public sitemap.
func indexablePageStatus(status string) bool {
	return status == "verified" || status == "deprecated"
}

// publicResourceURL resolves an application route against the configured public URL.
func publicResourceURL(publicURL string, segments ...string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(publicURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("invalid public URL")
	}

	basePath := strings.TrimRight(parsed.Path, "/")
	if len(segments) == 0 {
		parsed.Path = basePath + "/"
	} else {
		parts := append([]string{basePath}, segments...)
		parsed.Path = path.Join(parts...)
	}

	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return parsed.String(), nil
}
