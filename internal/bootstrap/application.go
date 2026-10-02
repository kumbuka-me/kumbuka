package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	appaccess "github.com/kumbuka-me/kumbuka/internal/application/access"
	appadministration "github.com/kumbuka-me/kumbuka/internal/application/administration"
	appgroups "github.com/kumbuka-me/kumbuka/internal/application/groups"
	appmedia "github.com/kumbuka-me/kumbuka/internal/application/media"
	appnavigation "github.com/kumbuka-me/kumbuka/internal/application/navigation"
	appnotifications "github.com/kumbuka-me/kumbuka/internal/application/notifications"
	apppages "github.com/kumbuka-me/kumbuka/internal/application/pages"
	appplugins "github.com/kumbuka-me/kumbuka/internal/application/plugins"
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
)

const rendererShutdownTimeout = 10 * time.Second

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

	// contentChangeWorker delivers committed page-source changes to plugin hooks.
	contentChangeWorker *pluginruntime.ContentChangeWorker
	// pluginUpdates coordinates scheduled and manual plugin catalog refreshes.
	pluginUpdates *appplugins.PluginUpdates
	// pluginUpdatesEnabled reports whether the background update scheduler should run.
	pluginUpdatesEnabled bool
}

// NewApplication composes application services around initialized process infrastructure.
func NewApplication(
	ctx context.Context,
	cfg flags.Config,
	infrastructure *Infrastructure,
	logger *slog.Logger,
	version, commit string,
) (*Application, error) {
	database := infrastructure.database
	secretCipher := infrastructure.cipher

	// Construct page mutation and collaboration capabilities that other workflows depend on.
	webhooks := appwebhooks.NewWebhooks(
		database,
		secretCipher,
		logger.With("component", "webhooks"),
		cfg.PublicURL,
	).WithUserDirectory(database)
	access := appaccess.NewAccess(database)
	mutations := apppages.NewMutations(database, access, database, logger, webhooks)
	presence := apppages.NewPresence(database, access)
	discussions := apppages.NewDiscussions(database, access, database, logger, webhooks)
	reviews := apppages.NewReviews(database, access, database, logger, webhooks)
	reviewDiscussions := apppages.NewReviewDiscussions(database, access, reviews, database, logger, webhooks)
	bulk := apppages.NewBulk(database, mutations, database, logger, webhooks)

	// Construct the remaining application capabilities around their narrow repository ports.
	administration := appadministration.NewAdministration(database)
	pageLookup := apppages.NewLookup(database, access)
	pageSearch := apppages.NewSearch(database, access)
	pageDirectory := apppages.NewDirectory(database, access)
	pageReports := apppages.NewReports(database, access)
	pagePersonal := apppages.NewPersonal(database, access)
	pageHistory := apppages.NewHistory(database, access)
	pageRender := apppages.NewRenderArtifacts(database)
	drafts := apppages.NewDrafts(database)
	groups := appgroups.NewGroups(database)
	knowledge := appsearch.NewKnowledge(database, access)
	notifications := appnotifications.NewNotifications(
		database,
		logger.With("component", "notifications"),
		webhooks,
	)
	mutations.WithNotifications(notifications)
	discussions.WithNotifications(notifications)
	reviews.WithNotifications(notifications)
	reviewDiscussions.WithNotifications(notifications)
	media := appmedia.NewMedia(database)
	navigation := appnavigation.NewNavigation(database, access)
	preferences := apppreferences.NewPreferences(database)
	recycleBin := apprecyclebin.NewRecycleBin(database)
	settings := appsettings.NewSettings(database, secretCipher, logger.With("component", "settings"))
	system := appsystem.NewSystem(database, logger.With("component", "system"), infrastructure.setupState)
	templates := apptemplates.NewTemplates(database)
	tokens := apptokens.NewTokens(database)
	users := appusers.NewUsers(database, credential.Passwords{}, logger.With("component", "users"))

	// Compose higher-level page workflows from the capabilities they coordinate.
	serverLogger := logger.With("component", "server")
	home := apppages.NewHomeQuery(database, drafts, access)
	editor := apppages.NewEditor(pageLookup, groups, templates)
	editorSave := apppages.NewEditorSave(mutations, drafts, templates, serverLogger)
	viewPage := apppages.NewView(database, access, reviews, serverLogger)

	// Construct API and browser authentication before starting plugin execution.
	bearerAuth := auth.NewBearer(database)
	browserConfig := newBrowserAuthConfig(cfg)
	browserConfig.SetupRequired = infrastructure.setupState.Required
	browserConfig.SetupCompleted = infrastructure.setupState.Complete
	browserAuth, err := auth.ConfigureBrowserAuth(ctx, browserConfig, database)
	if err != nil {
		return nil, fmt.Errorf("configure browser auth: %w", err)
	}

	// Construct the plugin runtime and attach its metrics provider.
	renderer, err := pluginruntime.NewRenderer(
		ctx,
		database,
		secretCipher,
		authenticatedPluginRequest,
		infrastructure.metrics,
		logger.With("component", "plugins"),
		version,
		commit,
	)
	if err != nil {
		return nil, fmt.Errorf("create plugin runtime: %w", err)
	}
	infrastructure.metrics.RegisterPluginProvider(renderer.PluginManager())

	// Inject runtime-derived content and icon capabilities into application services.
	iconCatalog := renderer.IconCatalog()
	content := pagecontent.New(renderer)
	navigation.WithIconValidator(iconCatalog)
	contentChangeWorker := pluginruntime.NewContentChangeWorker(
		database,
		renderer.PluginManager(),
		notifications,
		logger.With("component", "plugin-content-change-worker"),
	)
	mutations.WithIconValidator(iconCatalog).
		WithContentPreparer(content).
		WithContentChangeSink(contentChangeWorker)
	discussions.WithContentPreparer(content)
	reviewDiscussions.WithContentPreparer(content)
	settings.WithIconValidator(iconCatalog)
	templates.WithIconValidator(iconCatalog)

	// Construct optional background plugin update discovery.
	pluginUpdates := appplugins.NewPluginUpdates(
		pluginupdate.New(pluginupdate.DefaultCatalogURL),
		renderer.PluginManager(),
		notifications,
		cfg.PluginUpdateCheckInterval,
		logger.With("component", "plugin-updates"),
	)

	// Compose shared authenticated browser data after every dependency is available.
	browserContext := endpoint.NewBrowserContext(viewer.New(
		preferences,
		navigation,
		database,
		settings,
		knowledge,
		notifications,
		access,
	), renderer)

	return &Application{
		administration: administration,
		access:         access,
		groups:         groups,
		media:          media,
		navigation:     navigation,
		notifications:  notifications,
		preferences:    preferences,
		recycleBin:     recycleBin,
		knowledge:      knowledge,
		settings:       settings,
		system:         system,
		templates:      templates,
		tokens:         tokens,
		users:          users,
		webhooks:       webhooks,

		pageLookup:            pageLookup,
		pageSearch:            pageSearch,
		pageDirectory:         pageDirectory,
		pageReports:           pageReports,
		pagePersonal:          pagePersonal,
		pageHistory:           pageHistory,
		pageRender:            pageRender,
		drafts:                drafts,
		pageMutations:         mutations,
		pagePresence:          presence,
		pageDiscussions:       discussions,
		pageReviews:           reviews,
		pageReviewDiscussions: reviewDiscussions,
		pageBulk:              bulk,
		home:                  home,
		editor:                editor,
		editorSave:            editorSave,
		viewPage:              viewPage,

		browserAuth:    browserAuth,
		bearerAuth:     bearerAuth,
		browserContext: browserContext,
		renderer:       renderer,
		pluginAdmin:    appplugins.NewAdmin(renderer.PluginManager(), pluginUpdates),

		contentChangeWorker:  contentChangeWorker,
		pluginUpdates:        pluginUpdates,
		pluginUpdatesEnabled: cfg.PluginUpdateCheckInterval > 0,
	}, nil
}

// Start launches application-owned background work until ctx is canceled.
func (a *Application) Start(ctx context.Context) {
	if a == nil {
		return
	}

	go a.contentChangeWorker.Run(ctx)
	if a.pluginUpdatesEnabled {
		go a.pluginUpdates.Run(ctx)
	}
}

// Close releases application-owned runtime resources after cancellation.
func (a *Application) Close(logger *slog.Logger) {
	if a == nil || a.renderer == nil {
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
