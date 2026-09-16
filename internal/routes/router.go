package routes

import (
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/auth"
	"github.com/kumbuka-me/kumbuka/internal/handler"
	"github.com/kumbuka-me/kumbuka/internal/middleware"
	"github.com/kumbuka-me/kumbuka/internal/service"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// New constructs the application router and its authentication and authorization policies.
func New(
	appFS fs.FS,
	views *handler.Views,
	renderer *markdown.Renderer,
	browserAuth auth.BrowserAuth,
	bearerAuth auth.Authenticator,
	administrationUseCases *service.Administration,
	accessUseCases *service.Access,
	catalogUseCases *service.Catalog,
	draftUseCases *service.Drafts,
	groupUseCases *service.Groups,
	knowledgeUseCases *service.Knowledge,
	notificationUseCases *service.Notifications,
	mediaUseCases *service.Media,
	navigationUseCases *service.Navigation,
	pageUseCases *service.Pages,
	preferenceUseCases *service.Preferences,
	recycleBinUseCases *service.RecycleBin,
	settingsUseCases *service.Settings,
	systemUseCases *service.System,
	templateUseCases *service.Templates,
	tokenUseCases *service.Tokens,
	userUseCases *service.Users,
	webhookUseCases *service.Webhooks,
	viewDataUseCases *handler.ViewDataLoader,
	logger *slog.Logger,
	accessLog bool,
) http.Handler {
	mux := http.NewServeMux()

	// Authentication policies.
	// Browser routes authenticate exclusively through the configured browser
	// mechanism: local, trusted proxy, or OIDC session.
	browserAuthn := middleware.Authenticate(logger, browserAuth.Authenticator)
	// Media and API routes additionally accept personal access tokens. Browser
	// authentication remains available so the web frontend can call these
	// endpoints using its existing session without handling bearer tokens.
	mediaAuthn := middleware.Authenticate(logger, bearerAuth, browserAuth.Authenticator)
	apiAuthn := middleware.AuthenticateAPI(logger, bearerAuth, browserAuth.Authenticator)

	// Authorization policies.
	// Authentication establishes who the user is; these middleware restrict
	// routes further based on the authenticated user's role.
	adminAuthz := middleware.RequireRole("admin")
	editorAuthz := middleware.RequireRole("admin", "editor")

	addRoutes(
		mux,
		appFS,
		views,
		renderer,
		browserAuth,
		administrationUseCases,
		accessUseCases,
		catalogUseCases,
		draftUseCases,
		groupUseCases,
		knowledgeUseCases,
		notificationUseCases,
		mediaUseCases,
		navigationUseCases,
		pageUseCases,
		preferenceUseCases,
		recycleBinUseCases,
		settingsUseCases,
		systemUseCases,
		templateUseCases,
		tokenUseCases,
		userUseCases,
		webhookUseCases,
		viewDataUseCases,
		logger,
		browserAuthn,
		mediaAuthn,
		apiAuthn,
		adminAuthz,
		editorAuthz,
	)

	middlewares := []middleware.Middleware{middleware.RequestContext()}

	if accessLog {
		middlewares = append(middlewares, middleware.AccessLog(logger))
	}

	middlewares = append(
		middlewares,
		middleware.RecoverPanics(logger),
		middleware.RejectCrossSiteWrites(logger),
		middleware.SecurityHeaders(),
	)

	return middleware.Chain(mux, middlewares...)
}
