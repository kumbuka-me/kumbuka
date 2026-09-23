package webview

import (
	"html/template"
	"slices"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/domain"
	"github.com/kumbuka-me/kumbuka/pkg/markdown"
	"github.com/kumbuka-me/kumbuka/pkg/navigation"
	"github.com/kumbuka-me/kumbuka/pkg/plugin"
	"github.com/kumbuka-me/kumbuka/pkg/revision"
	"github.com/kumbuka-me/kumbuka/pkg/themes"
	"github.com/kumbuka-me/sdk/pluginpackage"
)

// Widget is one sanitized plugin widget rendered by a host template.
type Widget struct {
	// PluginID identifies the plugin associated with plugin widget view.
	PluginID string
	// ModuleID identifies the module associated with plugin widget view.
	ModuleID string
	// Width stores the width setting for plugin widget view.
	Width string
	// HTML stores the HTML value used by plugin widget view.
	HTML template.HTML
	// Actions contains host-rendered controls associated with the widget.
	Actions []WidgetAction
}

// WidgetAction contains one host-rendered widget action and command context.
type WidgetAction struct {
	// ID identifies the action within its widget.
	ID string
	// Kind selects link, dialog, or command presentation.
	Kind string
	// Label is the visible action text.
	Label string
	// URL is either the plugin-provided local target or the host-owned command endpoint.
	URL string
	// Icon is an optional host icon name.
	Icon string
	// Confirm is optional confirmation text for a command action.
	Confirm string
	// Surface identifies the widget placement that emitted the action.
	Surface string
	// PageSlug identifies the authorized current page when available.
	PageSlug string
	// Next is the local return path after a successful command.
	Next string
}

// WidgetPreference describes one plugin widget visibility control.
type WidgetPreference struct {
	// Key uniquely identifies the plugin widget.
	Key string
	// Label is the user-facing widget name.
	Label string
	// Surface is the user-facing placement label.
	Surface string
	// Description explains the widget when available.
	Description string
	// Visible reports whether the widget is currently shown.
	Visible bool
}

// ContentLanguageOption is one content language exposed by the administration UI.
type ContentLanguageOption struct {
	// Code is the persisted BCP 47 language tag.
	Code string
	// Label is the administrator-facing language name.
	Label string
}

// MediaItem contains image metadata plus its stable browser URL.
type MediaItem struct {
	// ID is the stable database identifier used in image URLs.
	ID int64 `json:"id"`
	// Filename is the sanitized image filename.
	Filename string `json:"filename"`
	// ContentType is the validated image MIME type.
	ContentType string `json:"content_type"`
	// SizeBytes is the stored image size in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// UploadedBy is the identifier of the user that uploaded the image.
	UploadedBy int64 `json:"uploaded_by"`
	// Uploader is the display name of the user that uploaded the image.
	Uploader string `json:"uploader"`
	// CreatedAt is the upload timestamp formatted by templates or clients.
	CreatedAt time.Time `json:"created_at"`
	// UsageCount is the number of Markdown references to the image across all pages.
	UsageCount int64 `json:"usage_count"`
	// URL is the stable authenticated browser URL for the image.
	URL string `json:"url"`
}

// PluginUpdate describes one compatible first-party update shown in plugin administration.
type PluginUpdate struct {
	// Version is the newer plugin version available from the catalog.
	Version string
	// ReleasedAt records when the catalog release was published.
	ReleasedAt time.Time
}

// PluginUpdateStatus describes catalog refresh state shown in plugin administration.
type PluginUpdateStatus struct {
	// Available reports whether the plugin update service is configured.
	Available bool
	// Automatic reports whether scheduled catalog checks are enabled.
	Automatic bool
	// LastAttempt records the latest scheduled or manual catalog refresh attempt.
	LastAttempt time.Time
	// LastSuccess records the latest successful catalog refresh.
	LastSuccess time.Time
	// LastError contains the latest catalog refresh failure, if any.
	LastError string
}

// ReviewDiffLine combines one revision diff line with feedback anchored at that source position.
type ReviewDiffLine struct {
	// Diff is the rendered line from the immutable reviewed revision comparison.
	Diff revision.DiffLine
	// AnchorSide selects the old or new source side used when creating feedback.
	AnchorSide string
	// AnchorLine is the one-based source line used when creating feedback.
	AnchorLine int
	// Comments contains feedback whose range starts at this source line.
	Comments []domain.PageReviewComment
}

// PageCommentThread groups one anchored root comment with every reply in its inline discussion.
type PageCommentThread struct {
	// RootID identifies the anchored root comment and its stable page fragment.
	RootID int64
	// Anchor is the selected rendered text associated with the inline discussion.
	Anchor string
	// Resolved reports whether the root inline discussion has been resolved.
	Resolved bool
	// Comments contains the root comment followed by replies in display order.
	Comments []domain.PageComment
}

// Layout contains presentation data for layout.
type Layout struct {
	// CurrentPage identifies contextual actions in shared browser chrome.
	CurrentPage *PageSummary
	// HasPageContents selects the reading-page layout.
	HasPageContents bool
	// SearchQuery is the current value of the global search input.
	SearchQuery string

	// PluginSettingsLinks contains installed plugins that expose settings or resources.
	PluginSettingsLinks []PluginSettingsLink

	// PluginFeatures contains enabled plugin and plugin-setting flags for browser UI decisions.
	PluginFeatures map[string]bool

	// PluginWidgetPreferences contains enabled plugin widgets and their user visibility.
	PluginWidgetPreferences []WidgetPreference

	// PluginModules is the current browser-module catalog embedded in the page.
	PluginModules template.JS

	// PluginStylesVersion fingerprints active plugin presentation styles for immutable browser caching.
	PluginStylesVersion string

	// Title is the page title displayed in the browser chrome.
	Title string

	// User is the authenticated user rendering the page.
	User domain.User

	// Preferences contains the current user's presentation preferences.
	Preferences domain.UserPreferences

	// TypographySize is the effective content typography preset after applying the application default.
	TypographySize string

	// SavedSearches contains named smart collections for the current user.
	SavedSearches []domain.SavedSearch

	// Notifications contains recent inbox items for the current user.
	Notifications []domain.Notification

	// UnreadNotifications is the current unread inbox count.
	UnreadNotifications int

	// SidebarWidgets contains sanitized plugin widgets above structural navigation.
	SidebarWidgets []Widget

	// AdminSection identifies the active administration navigation section.
	AdminSection string

	// ApplicationSettings contains mutable application-wide settings for administrators.
	ApplicationSettings domain.ApplicationSettings

	// EditorToolbar contains the resolved host-owned groups shared by both editor modes.
	EditorToolbar []plugin.ToolbarGroup

	// PagePathOptions contains existing page and folder locations available as parents.
	PagePathOptions []PagePathOption

	// NewPageParent is the active page path inherited by contextual new-page actions.
	NewPageParent string

	// Navigation is the slug-derived sidebar navigation tree.
	Navigation []navigation.Node

	// Version is the application version shown in the footer.
	Version string

	// AssetVersion fingerprints embedded browser assets for cache-safe URLs.
	AssetVersion string

	// Commit is the application commit shown in administration.
	Commit string

	// Runtime contains non-secret runtime configuration for administrators.
	Runtime RuntimeInfo

	// ThemeData is the JSON theme catalog consumed by the browser.
	ThemeData template.JS

	// Themes lists the theme titles available in settings.
	Themes []themes.Theme

	// ActiveTheme is the current user's validated theme title.
	ActiveTheme string

	// CanEdit reports whether the current user may create or edit pages.
	CanEdit bool
}

// PageSummary identifies the current page in shared browser chrome.
type PageSummary struct {
	// Slug is the active page path used by contextual actions.
	Slug string
	// Title is the page title shown in shared browser chrome.
	Title string
}

// CurrentPage projects a loaded page into contextual browser navigation.
func CurrentPage(page *domain.Page) *PageSummary {
	if page == nil {
		return nil
	}
	return &PageSummary{Slug: page.Slug, Title: page.Title}
}

// Screen is a typed browser model; embedding Layout supplies its marker.
type Screen interface{ browserScreen() }

// StatusView contains presentation data for a themed HTTP status page.
type StatusView struct {
	// Layout contains shared browser chrome and authenticated viewer context.
	Layout

	// StatusCode is the status code displayed above the title.
	StatusCode int
	// StatusMessage explains why the request could not be completed.
	StatusMessage string
	// StatusIcon is the host icon displayed above the status code.
	StatusIcon string
	// PrimaryLabel is the text of the primary action.
	PrimaryLabel string
	// PrimaryURL is the local target of the primary action.
	PrimaryURL string
	// PrimaryIcon is the host icon rendered in the primary action.
	PrimaryIcon string
	// SecondaryLabel is the text of the optional secondary action.
	SecondaryLabel string
	// SecondaryURL is the local target of the optional secondary action.
	SecondaryURL string
	// SecondaryIcon is the host icon rendered in the secondary action.
	SecondaryIcon string
}

// browserScreen marks shared layout and screen models as renderable.
func (Layout) browserScreen() {}

// AdminAuditView contains presentation data for admin audit view.
type AdminAuditView struct {
	// Layout contains shared browser presentation.
	Layout

	// AuditEvents contains recent administrative audit events.
	AuditEvents []domain.AuditEvent
}

// AdminBinView contains presentation data for admin bin view.
type AdminBinView struct {
	// Layout contains shared browser presentation.
	Layout

	// DeletedPages contains pages currently held in the recycle bin.
	DeletedPages []domain.DeletedPage
}

// AdminConfigurationView contains presentation data for admin configuration view.
type AdminConfigurationView struct {
	// Layout contains shared browser presentation.
	Layout

	// PDFHeaders contains administrator-safe PDF request-header metadata.
	PDFHeaders []domain.PDFHeader

	// ContentLanguages lists content languages available to administrators.
	ContentLanguages []ContentLanguageOption

	// Groups contains administratively managed user groups.
	Groups []domain.Group
}

// AdminExportsView contains presentation data for admin exports view.
type AdminExportsView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminPages contains all pages available for administrative export.
	AdminPages []domain.Page
}

// AdminGroupsView contains presentation data for admin groups view.
type AdminGroupsView struct {
	// Layout contains shared browser presentation.
	Layout

	// Groups contains administratively managed user groups.
	Groups []domain.Group
}

// AdminHealthView contains presentation data for admin health view.
type AdminHealthView struct {
	// Layout contains shared browser presentation.
	Layout

	// DocumentationHealth contains actionable documentation quality findings.
	DocumentationHealth domain.DocumentationHealth
}

// AdminImagesView contains presentation data for admin images view.
type AdminImagesView struct {
	// Layout contains shared browser presentation.
	Layout

	// Images contains uploaded media shown in settings or administration.
	Images []MediaItem

	// ImageQuery is the active filename/uploader filter for a managed image list.
	ImageQuery string

	// ImagesHasMore reports whether another managed image page is available.
	ImagesHasMore bool
}

// AdminNavigationView contains presentation data for admin navigation view.
type AdminNavigationView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminNavigation contains top-level navigation sections and their persisted icons.
	AdminNavigation []domain.NavigationItem
}

// AdminPagesView contains presentation data for admin pages view.
type AdminPagesView struct {
	// Layout contains shared browser presentation.
	Layout

	// Groups contains administratively managed user groups.
	Groups []domain.Group

	// PageStatuses contains lifecycle statuses available to page editors.
	PageStatuses []string

	// AdminPages contains all pages available for administrative export.
	AdminPages []domain.Page
}

// AdminPermissionsView contains presentation data for admin permissions view.
type AdminPermissionsView struct {
	// Layout contains shared browser presentation.
	Layout

	// Groups contains administratively managed user groups.
	Groups []domain.Group

	// PageAccessRules contains inherited path access rules for administrators.
	PageAccessRules []domain.PageAccessRule
}

// AdminPluginSettingsView contains presentation data for admin plugin settings view.
type AdminPluginSettingsView struct {
	// Layout contains shared browser presentation.
	Layout

	// PluginMessage contains the plugin message for view data.
	PluginMessage string

	// PluginSettings is the plugin currently shown on its dedicated settings page.
	PluginSettings *plugin.LoadedPlugin

	// PluginSettingsGroups contains typed singleton settings shown on the current plugin settings page.
	PluginSettingsGroups []plugin.SettingGroup

	// PluginSettingsResources contains structured settings shown on the current plugin settings page.
	PluginSettingsResources []PluginResource
}

// AdminPluginsView contains presentation data for admin plugins view.
type AdminPluginsView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminPlugins contains the admin plugins associated with view data.
	AdminPlugins []plugin.LoadedPlugin

	// PluginMessage contains the plugin message for view data.
	PluginMessage string

	// OpenPluginID identifies the plugin detail modal that should open after rendering.
	OpenPluginID string

	// PluginRequiredIDs identifies plugins protected by trusted operator policy.
	PluginRequiredIDs map[string]bool

	// PluginUpdates contains newer compatible first-party releases keyed by plugin ID.
	PluginUpdates map[string]*PluginUpdate

	// PluginUpdateStatus contains scheduled and manual catalog refresh state.
	PluginUpdateStatus PluginUpdateStatus

	// PluginCatalogUnavailable reports that the update catalog could not be checked for this render.
	PluginCatalogUnavailable bool

	// PluginHasSettings identifies plugins that expose administrator settings.
	PluginHasSettings map[string]bool

	// PluginREADMEs contains sanitized packaged documentation keyed by plugin ID.
	PluginREADMEs map[string]template.HTML
}

// AdminTagsView contains presentation data for admin tags view.
type AdminTagsView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminTags contains tags and page usage counts for administrators.
	AdminTags []domain.TagInfo
}

// AdminTemplatesView contains presentation data for admin templates view.
type AdminTemplatesView struct {
	// Layout contains shared browser presentation.
	Layout

	// Groups contains administratively managed user groups.
	Groups []domain.Group

	// PageTemplates contains reusable templates available to page authors.
	PageTemplates []domain.PageTemplate

	// PageStatuses contains lifecycle statuses available to page editors.
	PageStatuses []string
}

// AdminTokensView contains presentation data for admin tokens view.
type AdminTokensView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminUsers contains users and group memberships for administrators.
	AdminUsers []domain.AdminUser

	// AdminTokens contains all personal access tokens for administrators.
	AdminTokens []domain.APIToken
}

// AdminUsersView contains presentation data for admin users view.
type AdminUsersView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminUsers contains users and group memberships for administrators.
	AdminUsers []domain.AdminUser

	// PendingOIDCIdentities contains verified OIDC identities awaiting an administrator decision.
	PendingOIDCIdentities []domain.PendingOIDCIdentity

	// OIDCIdentityCount is the number of active external OIDC bindings.
	OIDCIdentityCount int

	// Groups contains administratively managed user groups.
	Groups []domain.Group
}

// AdminView contains presentation data for admin view.
type AdminView struct {
	// Layout contains shared browser presentation.
	Layout

	// AdminStats contains high-level persisted object counts for administrators.
	AdminStats domain.AdminStats

	// AttachmentCount is the number of uploaded attachments.
	AttachmentCount int64

	// DatabaseSizeBytes is the current PostgreSQL database size in bytes.
	DatabaseSizeBytes int64
}

// AdminWebhooksView contains presentation data for admin webhooks view.
type AdminWebhooksView struct {
	// Layout contains shared browser presentation.
	Layout

	// Webhooks contains outgoing administrator integrations.
	Webhooks []domain.Webhook

	// WebhookDeliveries contains recent outgoing delivery attempts.
	WebhookDeliveries []domain.WebhookDelivery

	// WebhookEvents contains supported event names.
	WebhookEvents []string

	// WebhookDraft provides enabled defaults for the create form.
	WebhookDraft domain.Webhook
}

// AuthenticationView contains presentation data for authentication view.
type AuthenticationView struct {
	// Layout contains shared browser presentation.
	Layout

	// AuthError contains a browser-facing authentication or setup validation error.
	AuthError string

	// AuthNext is the validated local path restored after interactive authentication.
	AuthNext string
}

// EditView contains presentation data for edit view.
type EditView struct {
	// Layout contains shared browser presentation.
	Layout

	// Page is the current page when one is being viewed or edited.
	Page *domain.Page

	// ContentLanguages lists content languages available to administrators.
	ContentLanguages []ContentLanguageOption

	// PageContentLanguage is the effective language for the current page/editor.
	PageContentLanguage string

	// Groups contains administratively managed user groups.
	Groups []domain.Group

	// PageTemplates contains reusable templates available to page authors.
	PageTemplates []domain.PageTemplate

	// PageStatuses contains lifecycle statuses available to page editors.
	PageStatuses []string

	// EditorTemplate is the selected template used to prefill a new page.
	EditorTemplate *domain.PageTemplate

	// EditorInitialSlug pre-fills a requested path for a new page.
	EditorInitialSlug string

	// EditorParentPath is the selected parent location in the guided page-path picker.
	EditorParentPath string

	// EditorPathSegment is the stable final path segment for edits and explicit new-page links.
	EditorPathSegment string
}

// GraphView contains presentation data for graph view.
type GraphView struct {
	// Layout contains shared browser presentation.
	Layout

	// Query is the active search query.
	Query string
}

// HomeView contains presentation data for home view.
type HomeView struct {
	// Layout contains shared browser presentation.
	Layout

	// HomeWidgets contains sanitized plugin widgets for the home dashboard.
	HomeWidgets []Widget
}

// PageView contains presentation data for page view.
type PageView struct {
	// Layout contains shared browser presentation.
	Layout

	// Page is the current page when one is being viewed or edited.
	Page *domain.Page

	// PageFavorite reports whether the current user has pinned the current page.
	PageFavorite bool

	// PageWatchScope is page or subtree when the current user watches this path.
	PageWatchScope string

	// PageReviewRequest is the active lightweight approval workflow item.
	PageReviewRequest domain.PageReviewRequest

	// CanReviewPage reports whether the current user may decide the pending review.
	CanReviewPage bool

	// CanManageReview reports whether the current user may edit or cancel the pending request.
	CanManageReview bool

	// ReviewGroups contains collaboration groups available as review targets.
	ReviewGroups []domain.Group

	// HTML is the sanitized rendered Markdown for the current page.
	HTML template.HTML

	// PageContents contains heading links for the current rendered page.
	PageContents []markdown.Heading

	// PageDetailWidgets contains sanitized plugin widgets for the page details surface.
	PageDetailWidgets []Widget

	// PluginPageActions contains host-rendered navigation actions contributed for the current page.
	PluginPageActions []plugin.PageActionContribution

	// PluginExporters contains active plugin-owned page download formats.
	PluginExporters []plugin.ExporterContribution

	// Comments contains page-level discussion items that are not anchored inline.
	Comments []domain.PageComment

	// InlineCommentThreads contains anchored discussion threads rendered beside page text.
	InlineCommentThreads []PageCommentThread

	// PageContentLanguage is the effective language for the current page/editor.
	PageContentLanguage string

	// PluginInspectors contains active plugin-owned reading-page inspection data.
	PluginInspectors []plugin.Inspector

	// PluginExportFields contains plugin-owned request-local export controls used by this page.
	PluginExportFields []plugin.ExportField
}

// ReviewView contains presentation data for review view.
type ReviewView struct {
	// Layout contains shared browser presentation.
	Layout

	// Page is the current page when one is being viewed or edited.
	Page *domain.Page

	// PageReviewRequest is the active lightweight approval workflow item.
	PageReviewRequest domain.PageReviewRequest

	// ReviewDiff contains the line-oriented diff and feedback for a dedicated review page.
	ReviewDiff []ReviewDiffLine

	// CanCommentReview reports whether the current actor may add review comments.
	CanCommentReview bool

	// CanSuggestReview reports whether the current actor may propose source changes.
	CanSuggestReview bool

	// CanApplyReviewSuggestions reports whether the current actor may apply pending suggestions.
	CanApplyReviewSuggestions bool

	// OpenReviewSuggestions is the number of unapplied suggestions on the current review.
	OpenReviewSuggestions int
}

// RevisionsView contains presentation data for revisions view.
type RevisionsView struct {
	// Layout contains shared browser presentation.
	Layout

	// Revisions contains revision history rendered in the on-demand history dialog.
	Revisions []revision.Revision

	// RevisionSlug is the page path used by revision history actions.
	RevisionSlug string
}

// SearchView contains presentation data for search view.
type SearchView struct {
	// Layout contains shared browser presentation.
	Layout

	// Pages contains the primary page collection for the current view.
	Pages []domain.Page

	// Query is the active search query.
	Query string
}

// SettingsView contains presentation data for settings view.
type SettingsView struct {
	// Layout contains shared browser presentation.
	Layout

	// Images contains uploaded media shown in settings or administration.
	Images []MediaItem

	// ImageQuery is the active filename/uploader filter for a managed image list.
	ImageQuery string

	// ImagesHasMore reports whether another managed image page is available.
	ImagesHasMore bool

	// UserTokens contains personal access tokens owned by the current user.
	UserTokens []domain.APIToken

	// Groups contains administratively managed user groups.
	Groups []domain.Group

	// LocalCredentialAuthenticated reports whether this request used a local session.
	LocalCredentialAuthenticated bool
}

// PluginSettingsLink is one installed plugin exposed in administration settings navigation.
type PluginSettingsLink struct {
	// ID is the stable plugin identifier used in the settings URL.
	ID string
	// Name is the human-readable plugin name.
	Name string
	// Icon is the validated icon used for this plugin in administration navigation.
	Icon string
	// Section is the administration section key used to highlight the active entry.
	Section string
}

// PluginResource contains one declarative plugin resource schema and its records.
type PluginResource struct {
	// Module contains the validated declarative resource schema.
	Module pluginpackage.Module
	// Records contains persisted records in deterministic key order.
	Records []plugin.ResourceRecord
}

// pageTemplateView contains template data for page template view.
type pageTemplateView struct {
	// PageTemplate embeds page template behavior in page template view.
	domain.PageTemplate
	// Groups contains the groups associated with page template view.
	Groups []domain.Group
	// PageStatuses contains the page statuses associated with page template view.
	PageStatuses []string
}

// webhookView contains template data for webhook view.
type webhookView struct {
	// Webhook embeds webhook behavior in webhook view.
	domain.Webhook
	// AvailableEvents contains the available events associated with webhook view.
	AvailableEvents []string
	// EncryptionKeyConfigured reports whether encryption key configured applies to webhook view.
	EncryptionKeyConfigured bool
}

// PagePathOption is one selectable parent path in the page editor.
type PagePathOption struct {
	// Slug is the normalized page path associated with page path option.
	Slug string
	// Label is the display label for page path option.
	Label string
}

// excludedPagePath reports whether slug is the excluded page or one of its descendants.
func excludedPagePath(slug, excludedSlug string) bool {
	return excludedSlug != "" && (slug == excludedSlug || strings.HasPrefix(slug, excludedSlug+"/"))
}

// PagePathOptions flattens the navigation tree into selectable parent paths.
func PagePathOptions(tree []navigation.Node, excludedSlug string) []PagePathOption {
	var options []PagePathOption
	var appendNodes func([]navigation.Node, []string)

	appendNodes = func(nodes []navigation.Node, ancestors []string) {
		for _, node := range nodes {
			if excludedPagePath(node.Slug, excludedSlug) {
				continue
			}

			labels := append(slices.Clone(ancestors), node.Title)
			options = append(options, PagePathOption{
				Slug:  node.Slug,
				Label: strings.Join(labels, " / "),
			})
			appendNodes(node.Children, labels)
		}
	}

	appendNodes(tree, nil)
	return options
}

// HasPagePathOption reports whether a parent path can be selected.
func HasPagePathOption(options []PagePathOption, slug string) bool {
	if slug == "" {
		return true
	}
	return slices.ContainsFunc(options, func(option PagePathOption) bool {
		return option.Slug == slug
	})
}

// AdminImportView presents an import result.
type AdminImportView struct {
	// Layout contains shared browser chrome and authenticated viewer context.
	Layout
	// Query preserves the submitted import source or filter value.
	Query string
}
