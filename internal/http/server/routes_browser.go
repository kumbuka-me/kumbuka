package server

import (
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/middleware"
)

// registerBrowserRoutes registers authenticated browser, media, preference, and notification endpoints.
func registerBrowserRoutes(mux *http.ServeMux, config Config) {
	browserAuthn := middleware.Authenticate(config.Logger, config.BrowserAuth.Authenticator)
	mediaAuthn := middleware.Authenticate(config.Logger, config.BearerAuth, config.BrowserAuth.Authenticator)

	mux.Handle("POST /auth/logout", browserAuthn(auth.Logout(config.BrowserAuth.Local)))
	mux.Handle("GET /{$}", browserAuthn(endpoint.Home(config.BrowserContext, config.Home, config.Renderer, config.Views)))
	mux.Handle("GET /search", browserAuthn(endpoint.Search(config.BrowserContext, config.PageSearch, config.Views)))
	mux.Handle("GET /graph", browserAuthn(endpoint.KnowledgeGraphPage(config.BrowserContext, config.Views)))
	mux.Handle("GET /p/{id}", browserAuthn(endpoint.PagePermalink(config.PageLookup, config.Logger)))
	mux.Handle(
		"GET /settings",
		browserAuthn(endpoint.Settings(config.BrowserContext, config.Users, config.Tokens, config.Media, config.BrowserAuth.Local, config.Views)),
	)

	mux.Handle("GET /media/{id}/{name...}", mediaAuthn(endpoint.ServeImage(config.Media, config.Logger)))
	mux.Handle("GET /attachments/{id}/{name...}", mediaAuthn(endpoint.ServeAttachment(config.Media, config.Logger)))

	mux.Handle("POST /settings/preferences", browserAuthn(endpoint.SavePreferences(config.Preferences, config.Renderer.PluginManager(), config.Views)))
	mux.Handle("POST /settings/local-password", browserAuthn(endpoint.ChangeLocalPassword(config.BrowserAuth.Local, config.Logger)))
	mux.Handle(
		"POST /settings/preferences/page-contents",
		browserAuthn(endpoint.SavePageContentsPreference(config.Preferences, config.Views)),
	)
	mux.Handle(
		"POST /settings/preferences/navigation-state",
		browserAuthn(endpoint.SaveNavigationState(config.Preferences, config.Logger)),
	)
	mux.Handle(
		"POST /settings/preferences/sidebar-width",
		browserAuthn(endpoint.SaveSidebarWidth(config.Preferences, config.Logger)),
	)
	mux.Handle("POST /settings/saved-searches", browserAuthn(endpoint.CreateSavedSearch(config.Knowledge, config.Logger)))
	mux.Handle(
		"POST /settings/saved-searches/{id}/delete",
		browserAuthn(endpoint.DeleteSavedSearch(config.Knowledge, config.Logger)),
	)
	mux.Handle("POST /settings/tokens", browserAuthn(endpoint.CreatePersonalToken(config.Tokens, config.Logger)))
	mux.Handle("DELETE /settings/tokens/{id}", browserAuthn(endpoint.DeletePersonalToken(config.Tokens, config.Logger)))
	mux.Handle("POST /notifications/{id}/open", browserAuthn(endpoint.OpenNotification(config.Notifications, config.Logger)))
}
