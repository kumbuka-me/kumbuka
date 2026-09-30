package server

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
)

// TestPageReviewSuggestionRoutesDoNotConflict verifies single and bulk apply patterns can coexist in ServeMux.
func TestPageReviewSuggestionRoutesDoNotConflict(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("POST /reviews/{id}/suggestions/apply/{commentID}/{slug...}", http.NotFoundHandler())
	mux.Handle("POST /reviews/{id}/suggestions/apply-all/{slug...}", http.NotFoundHandler())
}

// TestPageCommentSuggestionRouteDoesNotConflict verifies inline suggestion apply remains more specific than comment creation.
func TestPageCommentSuggestionRouteDoesNotConflict(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle("POST /page-comments/{slug...}", http.NotFoundHandler())
	mux.Handle("POST /page-comments/suggestions/apply/{id}/{slug...}", http.NotFoundHandler())
}

// setupRouteSystemRepository supplies the minimal System dependencies required by route tests.
type setupRouteSystemRepository struct{}

func (setupRouteSystemRepository) DatabaseSize(context.Context) (int64, error) { return 0, nil }
func (setupRouteSystemRepository) Ping(context.Context) error                  { return nil }
func (setupRouteSystemRepository) LogAudit(context.Context, int64, string, string, string, string) error {
	return nil
}

func TestSetupRoutesRegisteredOnlyWhenRequired(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name     string
		required bool
		want     string
	}{
		{name: "fresh install", required: true, want: "GET /setup"},
		{name: "configured install", required: false, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			system := appsystem.NewSystem(
				setupRouteSystemRepository{},
				slog.Default(),
				appsystem.NewSetupState(test.required),
			)
			mux := http.NewServeMux()
			registerSetupRoutes(mux, Config{AdministrationConfig: AdministrationConfig{System: system}})

			_, pattern := mux.Handler(httptest.NewRequest(http.MethodGet, "/setup", nil))
			if pattern != test.want {
				t.Fatalf("setup route pattern = %q, want %q", pattern, test.want)
			}
		})
	}
}
