package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/web"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type prefixSystemRepository struct{}

func (prefixSystemRepository) Ping(context.Context) error                  { return nil }
func (prefixSystemRepository) SetupRequired(context.Context) (bool, error) { return false, nil }
func (prefixSystemRepository) DatabaseSize(context.Context) (int64, error) { return 0, nil }
func (prefixSystemRepository) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

type prefixAuthenticator struct{}

func (prefixAuthenticator) Authenticate(*http.Request) (domain.User, error) {
	return domain.User{}, auth.ErrUnauthenticated
}

type prefixMetrics struct{}

func (prefixMetrics) Metrics() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "kumbuka_test 1\n") })
}
func (prefixMetrics) InstrumentHTTP(h http.Handler) http.Handler { return h }

func TestDeploymentPrefix(t *testing.T) {
	for _, prefix := range []string{"", "/kumbuka", "/pages"} {
		t.Run(prefix, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			views, err := webview.New(web.Assets, logger, "test", "test", nil, webview.RuntimeInfo{RoutePrefix: prefix})
			require.NoError(t, err)
			config := Config{
				InfrastructureConfig: InfrastructureConfig{RoutePrefix: prefix, Assets: web.Assets, Views: views, Renderer: markdown.NewWithRegistry(nil), Logger: logger, MetricsEnabled: true, Metrics: prefixMetrics{}},
				AuthenticationConfig: AuthenticationConfig{BrowserAuth: auth.BrowserAuth{Authenticator: prefixAuthenticator{}, Login: http.NotFoundHandler()}, BearerAuth: prefixAuthenticator{}},
				AdministrationConfig: AdministrationConfig{System: appsystem.NewSystem(prefixSystemRepository{}, slog.Default())},
			}
			handler := New(config)
			for _, path := range []string{"/healthz", "/metrics", "/assets/v-test/favicon.svg", "/plugins/runtime.js", "/sw.js"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", prefix+path, nil))
				assert.Equal(t, http.StatusOK, response.Code, path)
				if path == "/sw.js" {
					assert.Equal(t, prefix+"/", response.Header().Get("Service-Worker-Allowed"))
				}
				if prefix != "" {
					outside := httptest.NewRecorder()
					handler.ServeHTTP(outside, httptest.NewRequest("GET", path, nil))
					assert.Equal(t, http.StatusNotFound, outside.Code, path)
				}
			}
			for _, path := range []string{"/healthz", "/metrics"} {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("POST", prefix+path, nil))
				assert.Equal(t, http.StatusMethodNotAllowed, response.Code)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest("GET", prefix+"/pages/foo", nil))
			assert.Equal(t, http.StatusFound, response.Code)
			assert.Equal(t, prefix+"/auth/login?next=%2Fpages%2Ffoo", response.Header().Get("Location"))
			for _, path := range []string{"/assets", "/api", "/edit"} {
				redirect := httptest.NewRecorder()
				handler.ServeHTTP(redirect, httptest.NewRequest("GET", prefix+path, nil))
				assert.Equal(t, prefix+path+"/", redirect.Header().Get("Location"))
			}
			if prefix != "" {
				response = httptest.NewRecorder()
				handler.ServeHTTP(response, httptest.NewRequest("GET", prefix, nil))
				assert.Equal(t, http.StatusPermanentRedirect, response.Code)
				assert.Equal(t, prefix+"/", response.Header().Get("Location"))
			}
		})
	}
}
