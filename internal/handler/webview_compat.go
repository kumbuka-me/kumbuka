package handler

import (
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"

	"github.com/kumbuka-me/kumbuka/internal/webview"
	"github.com/kumbuka-me/kumbuka/pkg/icons"
	md "github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
)

// RuntimeInfo is retained while callers migrate to webview.RuntimeInfo.
type RuntimeInfo = webview.RuntimeInfo

// Views is retained while handlers migrate to webview.Views explicitly.
type Views = webview.Views

// ViewData is retained while handlers migrate to webview.Data explicitly.
type ViewData = webview.Data

// ViewDataLoader is retained while application wiring migrates to webview.Loader.
type ViewDataLoader = webview.Loader

// MediaItem is retained while browser-facing media responses migrate to webview.MediaItem.
type MediaItem = webview.MediaItem

type contentLanguageOption = webview.ContentLanguageOption
type pluginWidgetView = webview.Widget
type pluginWidgetActionView = webview.WidgetAction
type pluginWidgetPreferenceView = webview.WidgetPreference
type pluginResourceView = webview.PluginResource
type pagePathOption = webview.PagePathOption

// NewViews delegates construction to the webview package during the package split.
func NewViews(
	appFS fs.FS,
	logger *slog.Logger,
	version, commit string,
	availableThemes []themes.Theme,
	runtime RuntimeInfo,
	iconCatalog ...*icons.Catalog,
) (*Views, error) {
	return webview.New(appFS, logger, version, commit, availableThemes, runtime, iconCatalog...)
}

// NewViewDataLoader delegates shared presentation-data loading to webview.
func NewViewDataLoader(
	preferences preferenceService,
	navigation navigationService,
	catalog sidebarCatalogService,
	settings settingsService,
	savedSearches savedSearchReader,
	notifications notificationReader,
	access pageAccessReader,
	renderer *md.Renderer,
) *ViewDataLoader {
	return webview.NewLoader(preferences, navigation, catalog, settings, savedSearches, notifications, access, renderer)
}

func publicViewData(views *Views, title string) (ViewData, error) {
	return views.PublicData(title)
}

func render(views *Views, w http.ResponseWriter, page string, data ViewData) {
	views.Render(w, page, data)
}

func renderStatus(views *Views, w http.ResponseWriter, status int, page string, data ViewData) {
	views.RenderStatus(w, status, page, data)
}

func renderPublic(views *Views, w http.ResponseWriter, page string, data ViewData) {
	views.RenderPublic(w, page, data)
}

func renderFragment(views *Views, w http.ResponseWriter, page, name string, data ViewData) {
	views.RenderFragment(w, page, name, data)
}

func renderTemplate(views *Views, w http.ResponseWriter, page, name string, data ViewData) {
	views.RenderTemplate(w, page, name, data)
}

func renderTemplateStatus(views *Views, w http.ResponseWriter, status int, page, name string, data ViewData) {
	views.RenderDataStatus(w, status, page, name, data)
}

func renderTemplateDataStatus(views *Views, w http.ResponseWriter, status int, page, name string, data any) {
	views.RenderDataStatus(w, status, page, name, data)
}

func renderTemplateHTML(views *Views, page, name string, data ViewData) (template.HTML, error) {
	return views.RenderHTML(page, name, data)
}

func widgetViews(widgets []md.RenderedWidget, surface, pageSlug, next string) []pluginWidgetView {
	return webview.Widgets(widgets, surface, pageSlug, next)
}

func pagePathOptions(tree []navigation.Node, excludedSlug string) []pagePathOption {
	return webview.PagePathOptions(tree, excludedSlug)
}

func hasPagePathOption(options []pagePathOption, slug string) bool {
	return webview.HasPagePathOption(options, slug)
}
