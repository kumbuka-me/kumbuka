package endpoint

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containeroo/httpprefix"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type canonicalPageServiceStub struct {
	page       domain.Page
	alias      string
	pageByID   int
	aliasReads int
}

func (s *canonicalPageServiceStub) PageByIDFor(context.Context, domain.User, int64) (domain.Page, error) {
	s.pageByID++
	return s.page, nil
}

func (s *canonicalPageServiceStub) GetPageOrAliasFor(context.Context, domain.User, string) (domain.Page, string, error) {
	s.aliasReads++
	return s.page, s.alias, nil
}

func TestCanonicalPageID(t *testing.T) {
	t.Parallel()

	t.Run("accepts positive page identifier", func(t *testing.T) {
		t.Parallel()

		id, err := canonicalPageID("42")

		require.NoError(t, err)
		assert.Equal(t, int64(42), id)
	})

	t.Run("rejects invalid page identifier", func(t *testing.T) {
		t.Parallel()

		_, err := canonicalPageID("page")

		require.Error(t, err)
	})

	t.Run("rejects non-positive page identifier", func(t *testing.T) {
		t.Parallel()

		_, err := canonicalPageID("0")

		require.Error(t, err)
	})
}

func TestCanonicalPageRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	page := domain.Page{ID: 123, Slug: "getting-started/install"}

	for _, test := range []struct {
		name       string
		path       string
		id         string
		slug       string
		legacy     bool
		wantStatus int
		wantTarget string
		wantRender int
	}{
		{name: "short permalink", path: "/p/123", id: "123", wantStatus: http.StatusPermanentRedirect, wantTarget: "/p/123/getting-started/install"},
		{name: "canonical", path: "/p/123/getting-started/install", id: "123", slug: page.Slug, wantStatus: http.StatusOK, wantRender: 1},
		{name: "stale slug", path: "/p/123/old-name", id: "123", slug: "old-name", wantStatus: http.StatusPermanentRedirect, wantTarget: "/p/123/getting-started/install"},
		{name: "legacy current slug", path: "/pages/getting-started/install", slug: page.Slug, legacy: true, wantStatus: http.StatusPermanentRedirect, wantTarget: "/p/123/getting-started/install"},
		{name: "legacy alias", path: "/pages/old-install-location", slug: "old-install-location", legacy: true, wantStatus: http.StatusPermanentRedirect, wantTarget: "/p/123/getting-started/install"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service := &canonicalPageServiceStub{page: page, alias: "old-install-location"}
			renders := 0
			render := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { renders++; w.WriteHeader(http.StatusOK) })
			var handler http.Handler = CanonicalPage(service, logger, render)
			if test.legacy {
				handler = LegacyPage(service, logger)
			}
			handler = httpprefix.MountUnderPrefixWithOptions(handler, "/kumbuka", httpprefix.WithRedirectRewriting())
			request := httptest.NewRequest(http.MethodGet, "/kumbuka"+test.path, nil)
			request.SetPathValue("id", test.id)
			request.SetPathValue("slug", test.slug)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			assert.Equal(t, test.wantStatus, response.Code)
			assert.Equal(t, test.wantRender, renders)
			if test.wantTarget != "" {
				assert.Equal(t, "/kumbuka"+test.wantTarget, response.Header().Get("Location"))
			}
		})
	}
}

func TestCanonicalRedirectsDoNotInvokePageRenderPath(t *testing.T) {
	t.Parallel()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	service := &canonicalPageServiceStub{page: domain.Page{ID: 123, Slug: "guide/install"}}
	renders := 0
	render := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { renders++ })
	handler := CanonicalPage(service, logger, render)
	request := httptest.NewRequest(http.MethodGet, "/p/123/old-install", nil)
	request.SetPathValue("id", "123")
	request.SetPathValue("slug", "old-install")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusPermanentRedirect, response.Code)
	assert.Zero(t, renders, "canonicalization must stop before page-view and activity side effects")
}
