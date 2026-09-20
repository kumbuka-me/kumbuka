package server

import (
	"io/fs"
	"log/slog"
	"net/http"

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
	appwebhooks "github.com/kumbuka-me/kumbuka/internal/application/webhooks"
	"github.com/kumbuka-me/kumbuka/internal/http/auth"
	"github.com/kumbuka-me/kumbuka/internal/http/endpoint"
	"github.com/kumbuka-me/kumbuka/internal/http/routes"
	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
)

// Config contains the dependencies required to construct the HTTP adapter.
type Config struct {
	// Home loads dashboard page-list capabilities with application-owned access filtering.
	Home *apppages.HomeQuery
	// Editor loads create/edit page workflow data.
	Editor *apppages.Editor
	// EditorSave coordinates browser-editor page saves and draft cleanup.
	EditorSave *apppages.EditorSave
	// ViewPage loads authorized reading-page state.
	ViewPage *apppages.View
	// Assets contains the embedded web application assets served by HTTP endpoints.
	Assets fs.FS
	// Views renders HTML responses and exposes the shared icon catalog.
	Views *webview.Views
	// Renderer renders Markdown and owns the active plugin manager.
	Renderer *markdown.Renderer
	// PluginUpdates schedules, discovers, and downloads compatible first-party plugin releases.
	PluginUpdates *appplugins.PluginUpdates
	// BrowserAuth contains browser authentication handlers and identity resolution.
	BrowserAuth auth.BrowserAuth
	// BearerAuth authenticates API requests that use personal access tokens.
	BearerAuth auth.Authenticator
	// Administration provides administrator-facing application use cases.
	Administration *appadministration.Administration
	// Access provides page authorization and access-policy use cases.
	Access *appaccess.Access
	// PageLookup resolves direct page reads and aliases with actor authorization.
	PageLookup *apppages.Lookup
	// PageSearch provides actor-filtered page listing, search, and tag queries.
	PageSearch *apppages.Search
	// PageDirectory provides page aliases and inventory queries.
	PageDirectory *apppages.Directory
	// PageReports supplies authorized page data to reports and plugin capabilities.
	PageReports *apppages.Reports
	// PagePersonal owns actor-specific favorite and watch mutations.
	PagePersonal *apppages.Personal
	// PageHistory provides actor-authorized revision history.
	PageHistory *apppages.History
	// PageRender stores reusable page render artifacts.
	PageRender *apppages.RenderArtifacts
	// Drafts provides page-draft use cases.
	Drafts *apppages.Drafts
	// Groups provides group-management use cases.
	Groups *appgroups.Groups
	// Knowledge provides knowledge-graph and saved-search use cases.
	Knowledge *appsearch.Knowledge
	// Notifications provides notification use cases.
	Notifications *appnotifications.Notifications
	// Media provides image and attachment use cases.
	Media *appmedia.Media
	// Navigation provides navigation-tree and icon use cases.
	Navigation *appnavigation.Navigation
	// PageMutations owns core page create, update, move, delete, review, and revision restoration.
	PageMutations *apppages.Mutations
	// PagePresence owns collaborative editor presence.
	PagePresence *apppages.Presence
	// PageDiscussions owns page comments and inline suggestions.
	PageDiscussions *apppages.Discussions
	// PageReviews owns approval workflows.
	PageReviews *apppages.Reviews
	// PageReviewDiscussions owns review comments and suggestions.
	PageReviewDiscussions *apppages.ReviewDiscussions
	// PageBulk owns administrative bulk mutations and imports.
	PageBulk *apppages.Bulk
	// Preferences provides per-user preference use cases.
	Preferences *apppreferences.Preferences
	// RecycleBin provides deleted-page lifecycle use cases.
	RecycleBin *apprecyclebin.RecycleBin
	// Settings provides application-settings use cases.
	Settings *appsettings.Settings
	// System provides health and setup-state use cases.
	System *appsystem.System
	// Templates provides page-template use cases.
	Templates *apptemplates.Templates
	// Tokens provides personal and administrator token use cases.
	Tokens *apptokens.Tokens
	// Users provides user and external-identity use cases.
	Users *appusers.Users
	// Webhooks provides webhook configuration and delivery use cases.
	Webhooks *appwebhooks.Webhooks
	// BrowserContext loads shared browser context and presentation contributions.
	BrowserContext *endpoint.BrowserContext
	// Logger records request and endpoint diagnostics.
	Logger *slog.Logger
	// AccessLog enables request access logging when true.
	AccessLog bool
	// ReadOnly blocks state-changing application routes while preserving authentication flows.
	ReadOnly bool
}

// New constructs the fully routed HTTP application.
func New(config Config) http.Handler {
	router := routes.New(routes.Config{
		Views:       config.Views,
		BrowserAuth: config.BrowserAuth,
		BearerAuth:  config.BearerAuth,
		Logger:      config.Logger,
		AccessLog:   config.AccessLog,
		ReadOnly:    config.ReadOnly,
	})
	addRoutes(router, config)
	return router.Handler()
}
