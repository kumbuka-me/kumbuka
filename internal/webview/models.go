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

// Data contains the data shared by server-rendered Kumbuka templates.
type Data struct {
	// AdminPlugins contains the admin plugins associated with view data.
	AdminPlugins []plugin.LoadedPlugin
	// PluginMessage contains the plugin message for view data.
	PluginMessage string
	// OpenPluginID identifies the plugin detail modal that should open after rendering.
	OpenPluginID string
	// PluginRequiredIDs identifies plugins protected by trusted operator policy.
	PluginRequiredIDs map[string]bool
	// PluginUpdates contains newer compatible first-party releases keyed by plugin ID.
	PluginUpdates map[string]PluginUpdate
	// PluginCatalogUnavailable reports that the update catalog could not be checked for this render.
	PluginCatalogUnavailable bool
	// PluginHasSettings identifies plugins that expose administrator settings.
	PluginHasSettings map[string]bool
	// PluginREADMEs contains sanitized packaged documentation keyed by plugin ID.
	PluginREADMEs map[string]template.HTML
	// PluginFeatures contains enabled plugin and plugin-setting flags for browser UI decisions.
	PluginFeatures map[string]bool
	// PluginWidgetPreferences contains enabled plugin widgets and their user visibility.
	PluginWidgetPreferences []WidgetPreference
	// PluginModules is the current browser-module catalog embedded in the page.
	PluginModules template.JS
	// PluginStylesVersion fingerprints active plugin presentation styles for immutable browser caching.
	PluginStylesVersion string
	// PluginResources contains generic plugin-owned administrative record collections keyed by plugin ID.
	PluginResources map[string][]PluginResource
	// Title is the page title displayed in the browser chrome.
	Title string
	// User is the authenticated user rendering the page.
	User domain.User
	// Preferences contains the current user's presentation preferences.
	Preferences domain.UserPreferences
	// TypographySize is the effective content typography preset after applying the application default.
	TypographySize string
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
	// Pages contains the primary page collection for the current view.
	Pages []domain.Page
	// SavedSearches contains named smart collections for the current user.
	SavedSearches []domain.SavedSearch
	// Notifications contains recent inbox items for the current user.
	Notifications []domain.Notification
	// UnreadNotifications is the current unread inbox count.
	UnreadNotifications int
	// HomeWidgets contains sanitized plugin widgets for the home dashboard.
	HomeWidgets []Widget
	// SidebarWidgets contains sanitized plugin widgets above structural navigation.
	SidebarWidgets []Widget
	// PageDetailWidgets contains sanitized plugin widgets for the page details surface.
	PageDetailWidgets []Widget
	// PluginPageActions contains host-rendered navigation actions contributed for the current page.
	PluginPageActions []plugin.PageActionContribution
	// PluginExporters contains active plugin-owned page download formats.
	PluginExporters []plugin.ExporterContribution
	// Comments contains anchored discussion items for the current page.
	Comments []domain.PageComment
	// Revisions contains revision history rendered in the on-demand history dialog.
	Revisions []revision.Revision
	// RevisionSlug is the page path used by revision history actions.
	RevisionSlug string
	// Images contains uploaded media shown in settings or administration.
	Images []MediaItem
	// ImageQuery is the active filename/uploader filter for a managed image list.
	ImageQuery string
	// ImagesHasMore reports whether another managed image page is available.
	ImagesHasMore bool
	// UserTokens contains personal access tokens owned by the current user.
	UserTokens []domain.APIToken
	// AdminSection identifies the active administration navigation section.
	AdminSection string
	// AdminStats contains high-level persisted object counts for administrators.
	AdminStats domain.AdminStats
	// ApplicationSettings contains mutable application-wide settings for administrators.
	ApplicationSettings domain.ApplicationSettings
	// PDFHeaders contains administrator-safe PDF request-header metadata.
	PDFHeaders []domain.PDFHeader
	// DocumentationHealth contains actionable documentation quality findings.
	DocumentationHealth domain.DocumentationHealth
	// ContentLanguages lists content languages available to administrators.
	ContentLanguages []ContentLanguageOption
	// PageContentLanguage is the effective language for the current page/editor.
	PageContentLanguage string
	// AdminUsers contains users and group memberships for administrators.
	AdminUsers []domain.AdminUser
	// PendingOIDCIdentities contains verified OIDC identities awaiting an administrator decision.
	PendingOIDCIdentities []domain.PendingOIDCIdentity
	// OIDCIdentityCount is the number of active external OIDC bindings.
	OIDCIdentityCount int
	// Groups contains administratively managed user groups.
	Groups []domain.Group
	// PageTemplates contains reusable templates available to page authors.
	PageTemplates []domain.PageTemplate
	// PageAccessRules contains inherited path access rules for administrators.
	PageAccessRules []domain.PageAccessRule
	// Webhooks contains outgoing administrator integrations.
	Webhooks []domain.Webhook
	// WebhookDeliveries contains recent outgoing delivery attempts.
	WebhookDeliveries []domain.WebhookDelivery
	// WebhookEvents contains supported event names.
	WebhookEvents []string
	// WebhookDraft provides enabled defaults for the create form.
	WebhookDraft domain.Webhook
	// PluginInspectors contains active plugin-owned reading-page inspection data.
	PluginInspectors []plugin.Inspector
	// PluginExportFields contains plugin-owned request-local export controls used by this page.
	PluginExportFields []plugin.ExportField
	// EditorInserts contains active plugin-owned editor actions.
	EditorInserts []plugin.EditorInsertContribution
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
	// PagePathOptions contains existing page and folder locations available as parents.
	PagePathOptions []PagePathOption
	// NewPageParent is the active page path inherited by contextual new-page actions.
	NewPageParent string
	// AdminTags contains tags and page usage counts for administrators.
	AdminTags []domain.TagInfo
	// AdminTokens contains all personal access tokens for administrators.
	AdminTokens []domain.APIToken
	// AuditEvents contains recent administrative audit events.
	AuditEvents []domain.AuditEvent
	// AdminPages contains all pages available for administrative export.
	AdminPages []domain.Page
	// DeletedPages contains pages currently held in the recycle bin.
	DeletedPages []domain.DeletedPage
	// AdminNavigation contains top-level navigation sections and their persisted icons.
	AdminNavigation []domain.NavigationItem
	// Tags contains tags exposed by the current view.
	Tags []string
	// Query is the active search query.
	Query string
	// AuthError contains a browser-facing authentication or setup validation error.
	AuthError string
	// AuthNext is the validated local path restored after interactive authentication.
	AuthNext string
	// LocalCredentialAuthenticated reports whether this request used a local session.
	LocalCredentialAuthenticated bool
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

// PagePathOptions flattens the navigation tree into selectable parent paths.
func PagePathOptions(tree []navigation.Node, excludedSlug string) []PagePathOption {
	var options []PagePathOption
	var appendNodes func([]navigation.Node, []string)

	appendNodes = func(nodes []navigation.Node, ancestors []string) {
		for _, node := range nodes {
			if excludedSlug != "" &&
				(node.Slug == excludedSlug || strings.HasPrefix(node.Slug, excludedSlug+"/")) {
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
