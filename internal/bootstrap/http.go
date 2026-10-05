package bootstrap

import (
	"io/fs"
	"log/slog"

	"github.com/kumbuka-me/kumbuka/internal/flags"
	httpresponse "github.com/kumbuka-me/kumbuka/internal/http/response"
	httpserver "github.com/kumbuka-me/kumbuka/internal/http/server"
	"github.com/kumbuka-me/kumbuka/internal/runtimeinfo"
	"github.com/kumbuka-me/kumbuka/internal/webview"
)

// NewHTTPConfig constructs the passive HTTP adapter from the completed application graph.
func NewHTTPConfig(
	appFS fs.FS,
	cfg flags.Config,
	infrastructure *Infrastructure,
	application *Application,
	logger *slog.Logger,
	version, commit string,
) (httpserver.Config, error) {
	views, err := webview.New(
		appFS,
		logger,
		version,
		commit,
		infrastructure.themes,
		runtimeinfo.New(cfg, infrastructure.cipher.Configured()),
		application.renderer.IconCatalog(),
	)
	if err != nil {
		return httpserver.Config{}, err
	}
	views.WithRenderErrorHandler(httpresponse.InternalServerError)

	return httpserver.Config{
		InfrastructureConfig: httpserver.InfrastructureConfig{
			RoutePrefix:            cfg.RoutePrefix,
			Assets:                 appFS,
			Views:                  views,
			Renderer:               application.renderer,
			Logger:                 logger,
			AccessLog:              cfg.AccessLog,
			ReadOnly:               cfg.ReadOnly,
			MetricsEnabled:         !cfg.DisableMetrics,
			Metrics:                infrastructure.metrics,
			PerformanceDiagnostics: cfg.PerformanceDiagnostics,
		},

		AuthenticationConfig: httpserver.AuthenticationConfig{
			BrowserAuth: application.browserAuth,
			BearerAuth:  application.bearerAuth,
		},

		BrowserConfig: httpserver.BrowserConfig{
			BrowserContext: application.browserContext,
			Preferences:    application.preferences,
			Knowledge:      application.knowledge,
			Notifications:  application.notifications,
		},

		AdministrationConfig: httpserver.AdministrationConfig{
			Administration: application.administration,
			PluginAdmin:    application.pluginAdmin,
			Groups:         application.groups,
			Settings:       application.settings,
			System:         application.system,
			Templates:      application.templates,
			Tokens:         application.tokens,
			Users:          application.users,
			Webhooks:       application.webhooks,
			Media:          application.media,
			Navigation:     application.navigation,
			RecycleBin:     application.recycleBin,
		},

		PageQueryConfig: httpserver.PageQueryConfig{
			Access:        application.access,
			PageLookup:    application.pageLookup,
			PageSearch:    application.pageSearch,
			PageDirectory: application.pageDirectory,
			PageReports:   application.pageReports,
			PagePersonal:  application.pagePersonal,
			PageHistory:   application.pageHistory,
			PageRender:    application.pageRender,
			Drafts:        application.drafts,
		},

		PageWorkflowConfig: httpserver.PageWorkflowConfig{
			PageMutations:         application.pageMutations,
			PagePresence:          application.pagePresence,
			PageDiscussions:       application.pageDiscussions,
			PageReviews:           application.pageReviews,
			PageReviewDiscussions: application.pageReviewDiscussions,
			PageBulk:              application.pageBulk,
			Home:                  application.home,
			Editor:                application.editor,
			EditorSave:            application.editorSave,
			ViewPage:              application.viewPage,
		},
	}, nil
}
