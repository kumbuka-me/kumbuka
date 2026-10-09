package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	appadministration "github.com/kumbuka-me/kumbuka/internal/application/administration"
	appgroups "github.com/kumbuka-me/kumbuka/internal/application/groups"
	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	appnavigation "github.com/kumbuka-me/kumbuka/internal/application/navigation"
	appnotifications "github.com/kumbuka-me/kumbuka/internal/application/notifications"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
	appportablearchive "github.com/kumbuka-me/kumbuka/internal/application/portablearchive"
	apppreferences "github.com/kumbuka-me/kumbuka/internal/application/preferences"
	apprecyclebin "github.com/kumbuka-me/kumbuka/internal/application/recyclebin"
	appsearch "github.com/kumbuka-me/kumbuka/internal/application/search"
	appsettings "github.com/kumbuka-me/kumbuka/internal/application/settings"
	appsystem "github.com/kumbuka-me/kumbuka/internal/application/system"
	apptemplates "github.com/kumbuka-me/kumbuka/internal/application/templates"
	apptokens "github.com/kumbuka-me/kumbuka/internal/application/tokens"
	appusers "github.com/kumbuka-me/kumbuka/internal/application/users"
	"github.com/kumbuka-me/kumbuka/internal/application/viewer"
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/internal/credential"
	"github.com/kumbuka-me/kumbuka/internal/flags"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/pagecontent"
	"github.com/kumbuka-me/kumbuka/internal/pluginruntime"
	"github.com/kumbuka-me/kumbuka/internal/pluginupdate"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/plugins"
)

const rendererShutdownTimeout = 10 * time.Second

// applicationWorker runs application-owned background work until its context is canceled.
type applicationWorker interface {
	Run(context.Context)
}

// Application contains the composed application graph and owned background runtime.
type Application struct {
	// administration provides administrator-facing application queries.
	administration *appadministration.Administration
	// access provides page authorization and access-policy use cases.
	access *appaccess.Access
	// groups provides group-management use cases.
	groups *appgroups.Groups
	// media provides image and attachment use cases.
	media *appmedia.Media
	// navigation provides navigation-tree and icon use cases.
	navigation *appnavigation.Navigation
	// notifications provides notification queries and mutations.
	notifications *appnotifications.Notifications
	// preferences provides per-user preference use cases.
	preferences *apppreferences.Preferences
	// recycleBin provides deleted-page lifecycle use cases.
	recycleBin *apprecyclebin.RecycleBin
	// knowledge provides knowledge-graph and saved-search use cases.
	knowledge *appsearch.Knowledge
	// settings provides application-settings use cases.
	settings *appsettings.Settings
	// system provides health and setup-state use cases.
	system *appsystem.System
	// templates provides page-template use cases.
	templates *apptemplates.Templates
	// tokens provides personal and administrator token use cases.
	tokens *apptokens.Tokens
	// users provides user and external-identity use cases.
	users *appusers.Users
	// webhooks provides webhook configuration and delivery use cases.
	webhooks *appwebhooks.Webhooks

	// pageLookup resolves direct page reads and aliases with actor authorization.
	pageLookup *apppages.Lookup
	// pageSearch provides actor-filtered page listing, search, and tag queries.
	pageSearch *apppages.Search
	// pageDirectory provides page aliases and inventory queries.
	pageDirectory *apppages.Directory
	// pageReports supplies authorized page data to reports and plugin capabilities.
	pageReports *apppages.Reports
	// pagePersonal owns actor-specific favorite and watch mutations.
	pagePersonal *apppages.Personal
	// pageHistory provides actor-authorized revision history.
	pageHistory *apppages.History
	// pageRender stores reusable page render artifacts.
	pageRender *apppages.RenderArtifacts
	// drafts provides page-draft use cases.
	drafts *apppages.Drafts
	// pageMutations owns core page create, update, move, delete, review, and restoration workflows.
	pageMutations *apppages.Mutations
	// pagePresence owns collaborative editor presence.
	pagePresence *apppages.Presence
	// pageDiscussions owns page comments and inline suggestions.
	pageDiscussions *apppages.Discussions
	// pageReviews owns approval workflows.
	pageReviews *apppages.Reviews
	// pageReviewDiscussions owns review comments and suggestions.
	pageReviewDiscussions *apppages.ReviewDiscussions
	// pageBulk owns administrative bulk page mutations and imports.
	pageBulk *apppages.Bulk
	// portableImport restores Kumbuka archives inside one explicit PostgreSQL transaction scope.
	portableImport *appportablearchive.Importer
	// home loads dashboard page-list capabilities with access filtering.
	home *apppages.HomeQuery
	// editor loads create and edit page workflow data.
	editor *apppages.Editor
	// editorSave coordinates browser-editor saves and draft cleanup.
	editorSave *apppages.EditorSave
	// viewPage loads authorized reading-page state.
	viewPage *apppages.View

	// browserAuth contains browser authentication handlers and identity resolution.
	browserAuth auth.BrowserAuth
	// bearerAuth authenticates API requests that use personal access tokens.
	bearerAuth auth.Authenticator
	// browserContext loads shared authenticated browser context and plugin contributions.
	browserContext *endpoint.BrowserContext
	// renderer renders Markdown and owns the active plugin manager.
	renderer *markdown.Renderer
	// pluginAdmin coordinates plugin lifecycle and catalog operations.
	pluginAdmin *appplugins.Admin
	// renderRebuilds coordinates application-owned background render rebuilds.
	renderRebuilds *pagecontent.Rebuilds

	// backgroundWorkers contains process-lifetime application workers configured before Start.
	backgroundWorkers []applicationWorker
	// backgroundCancel stops application-owned workers independently of the parent process context.
	backgroundCancel context.CancelFunc
	// backgroundGroup joins every application-owned worker before runtime resources close.
	backgroundGroup sync.WaitGroup
}

// NewApplication composes application services around initialized process infrastructure.
func NewApplication(
	ctx context.Context,
	cfg flags.Config,
	infrastructure *Infrastructure,
	logger *slog.Logger,
	version, commit string,
) (*Application, error) {
	application := &Application{}
	application.composeUseCases(cfg, infrastructure, logger)

	if err := application.configureAuthentication(ctx, cfg, infrastructure); err != nil {
		return nil, err
	}
	if err := application.configurePluginRuntime(ctx, cfg, infrastructure, logger, version, commit); err != nil {
		return nil, err
	}

	application.composeBrowserContext(infrastructure)
	return application, nil
}

// composeUseCases constructs persistence-backed application services and their application-level dependencies.
func (a *Application) composeUseCases(cfg flags.Config, infrastructure *Infrastructure, logger *slog.Logger) {
	database := infrastructure.database
	secretCipher := infrastructure.cipher

	// Page mutation and collaboration services form the dependency spine for
	// notifications, browser workflows, and plugin content processing.
	a.webhooks = appwebhooks.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	).WithUserDirectory(database)

	a.access = appaccess.NewAccess(database)
	a.navigation = appnavigation.NewNavigation(database, a.access)

	pagesLogger := logger.With("component", "pages")
	a.pageMutations = apppages.NewMutations(database, a.access, database, pagesLogger, a.navigation, a.webhooks)
	a.pagePresence = apppages.NewPresence(database, a.access)
	a.pageDiscussions = apppages.NewDiscussions(database, a.access, database, pagesLogger, a.webhooks)
	a.pageReviews = apppages.NewReviews(database, a.access, database, pagesLogger, a.webhooks)
	a.pageReviewDiscussions = apppages.NewReviewDiscussions(database, a.access, a.pageReviews, database, pagesLogger, a.webhooks)
	a.pageBulk = apppages.NewBulk(database, a.pageMutations, database, pagesLogger, a.webhooks)

	// Narrow query and administration services depend only on their repository ports.
	a.administration = appadministration.NewAdministration(database)
	a.pageLookup = apppages.NewLookup(database, a.access)
	a.pageSearch = apppages.NewSearch(database, a.access)
	a.pageDirectory = apppages.NewDirectory(database, a.access)
	a.pageReports = apppages.NewReports(database, a.access)
	a.pagePersonal = apppages.NewPersonal(database, a.access)
	a.pageHistory = apppages.NewHistory(database, a.access)
	a.pageRender = apppages.NewRenderArtifacts(database)
	a.drafts = apppages.NewDrafts(database)
	a.groups = appgroups.NewGroups(database)
	a.knowledge = appsearch.NewKnowledge(database, a.access)
	a.notifications = appnotifications.NewNotifications(
		database,
		logger.With("component", "notifications"),
		a.webhooks,
	)
	a.media = appmedia.NewMedia(database)
	a.portableImport = appportablearchive.NewImporter(
		newPortableImportTransactionRunner(database),
		a.pageMutations,
		a.pageBulk,
		a.navigation,
	)
	a.preferences = apppreferences.NewPreferences(database)
	a.recycleBin = apprecyclebin.NewRecycleBin(database, a.navigation)
	a.settings = appsettings.NewSettings(database, secretCipher, logger.With("component", "settings"))
	a.system = appsystem.NewSystem(database, logger.With("component", "system"), infrastructure.setupState)
	a.templates = apptemplates.NewTemplates(database)
	a.tokens = apptokens.NewTokens(database)
	a.users = appusers.NewUsers(database, credential.Passwords{}, logger.With("component", "users"))

	// Mutation services emit notifications only after the notification service exists.
	a.pageMutations.WithNotifications(a.notifications)
	a.pageDiscussions.WithNotifications(a.notifications)
	a.pageReviews.WithNotifications(a.notifications)
	a.pageReviewDiscussions.WithNotifications(a.notifications)

	// Higher-level browser workflows coordinate the already-constructed page services.
	a.home = apppages.NewHomeQuery(database, a.drafts, a.access)
	a.editor = apppages.NewEditor(a.pageLookup, a.groups, a.templates)
	a.editorSave = apppages.NewEditorSave(a.pageMutations, a.drafts, a.templates, pagesLogger)
	a.viewPage = apppages.NewView(database, a.access, a.pageReviews, pagesLogger)
}

// configureAuthentication constructs API and browser authentication after repository-backed settings are available.
func (a *Application) configureAuthentication(ctx context.Context, cfg flags.Config, infrastructure *Infrastructure) error {
	database := infrastructure.database
	a.bearerAuth = auth.NewBearer(database)

	browserConfig := newBrowserAuthConfig(cfg)
	browserConfig.SetupRequired = infrastructure.setupState.Required
	browserConfig.SetupCompleted = infrastructure.setupState.Complete

	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserConfig, database)
	if err != nil {
		return fmt.Errorf("configure browser auth: %w", err)
	}
	a.browserAuth = browserAuth
	return nil
}

// configurePluginRuntime constructs plugin execution, wires runtime-derived capabilities, and prepares background workers.
func (a *Application) configurePluginRuntime(
	ctx context.Context,
	cfg flags.Config,
	infrastructure *Infrastructure,
	logger *slog.Logger,
	version, commit string,
) error {
	database := infrastructure.database
	renderer, err := pluginruntime.NewRenderer(
		ctx,
		database,
		plugins.Distribution{},
		infrastructure.cipher,
		authenticatedPluginRequest,
		infrastructure.metrics,
		logger.With("component", "plugins"),
		version,
		commit,
	)
	if err != nil {
		return fmt.Errorf("create plugin runtime: %w", err)
	}
	a.renderer = renderer
	infrastructure.metrics.RegisterPluginProvider(renderer.PluginManager())

	iconCatalog := renderer.IconCatalog()
	content := pagecontent.New(renderer)
	a.navigation.WithIconValidator(iconCatalog)
	a.pageMutations.WithIconValidator(iconCatalog).
		WithContentPreparer(content)
	a.pageDiscussions.WithContentPreparer(content)
	a.pageReviewDiscussions.WithContentPreparer(content)
	a.settings.WithIconValidator(iconCatalog)
	a.templates.WithIconValidator(iconCatalog)

	contentChangeWorker := pluginruntime.NewContentChangeWorker(
		database,
		renderer.PluginManager(),
		a.notifications,
		logger.With("component", "plugin-content-change-worker"),
	)
	a.pageMutations.WithContentChangeSink(contentChangeWorker)
	a.backgroundWorkers = append(a.backgroundWorkers, contentChangeWorker)

	pluginUpdates := appplugins.NewPluginUpdates(
		pluginupdate.New(pluginupdate.DefaultCatalogURL),
		renderer.PluginManager(),
		a.notifications,
		cfg.PluginUpdateCheckInterval,
		logger.With("component", "plugin-updates"),
	)
	a.pluginAdmin = appplugins.NewAdmin(renderer.PluginManager(), pluginUpdates)
	if cfg.PluginUpdateCheckInterval > 0 {
		a.backgroundWorkers = append(a.backgroundWorkers, pluginUpdates)
	}

	a.renderRebuilds = pagecontent.NewRebuilds(
		a.pageDirectory,
		a.pageReports,
		a.pageRender,
		a.renderer,
		logger.With("component", "page-render-rebuilds"),
		cfg.RoutePrefix,
	)
	a.backgroundWorkers = append(a.backgroundWorkers, a.renderRebuilds)
	return nil
}

// composeBrowserContext builds shared authenticated browser data after every contributing service is configured.
func (a *Application) composeBrowserContext(infrastructure *Infrastructure) {
	database := infrastructure.database
	a.browserContext = endpoint.NewBrowserContext(viewer.New(
		a.preferences,
		a.navigation,
		database,
		a.settings,
		a.knowledge,
		a.notifications,
		a.access,
	), a.renderer)
}

// Start launches application-owned background work until ctx or Close cancels it.
func (a *Application) Start(ctx context.Context) {
	if a == nil || len(a.backgroundWorkers) == 0 {
		return
	}

	workerContext, cancel := context.WithCancel(ctx)
	a.backgroundCancel = cancel
	for _, worker := range a.backgroundWorkers {
		a.backgroundGroup.Go(func() { worker.Run(workerContext) })
	}
}

// Close stops application-owned background work before releasing runtime resources.
func (a *Application) Close(logger *slog.Logger) {
	if a == nil {
		return
	}

	if a.backgroundCancel != nil {
		a.backgroundCancel()
	}
	a.backgroundGroup.Wait()

	if a.renderer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), rendererShutdownTimeout)
	defer cancel()

	if err := a.renderer.Close(ctx); err != nil {
		logger.Error(
			"close markdown renderer",
			"event", "markdown_renderer_close_failed",
			"error", err,
		)
	}
}

// newBrowserAuthConfig maps deployment configuration onto the authentication boundary.
func newBrowserAuthConfig(cfg flags.Config) auth.BrowserConfig {
	return auth.BrowserConfig{
		ModeOverride: cfg.AuthModeOverride,
		TrustedProxy: auth.TrustedProxyHeaders{
			Username:    cfg.TrustedUsernameHeaders,
			Email:       cfg.TrustedEmailHeaders,
			DisplayName: cfg.TrustedDisplayNameHeaders,
			Groups:      cfg.TrustedGroupHeaders,
			AdminGroup:  cfg.TrustedAdminGroup,
		},
		OIDC: auth.OIDCConfig{
			ClientID:      cfg.OIDCClientID,
			ClientSecret:  cfg.OIDCClientSecret,
			Issuer:        cfg.OIDCIssuer,
			SessionSecret: cfg.OIDCSessionSecret,
			PublicURL:     cfg.PublicURL,
			GroupClaim:    cfg.OIDCGroupClaim,
			AdminGroup:    cfg.OIDCAdminGroup,
		},
		LocalLoginEnabled:             cfg.LocalLogin,
		AllowUserRegistrationOverride: cfg.AllowUserRegistrationOverride,
	}
}

// authenticatedPluginRequest reports whether the current plugin invocation belongs to an authenticated user.
func authenticatedPluginRequest(ctx context.Context) bool {
	user, ok := auth.ContextUser(ctx)
	return ok && user.ID > 0
}
