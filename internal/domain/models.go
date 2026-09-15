package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/internal/pluginusage"
)

var (
	// ErrNotFound indicates that a requested domain object does not exist.
	ErrNotFound = errors.New("not found")
	// ErrAlreadyExists indicates that a unique domain object already exists.
	ErrAlreadyExists = errors.New("already exists")
	// ErrForbidden indicates that a requested domain mutation is not permitted.
	ErrForbidden = errors.New("forbidden")
	// ErrRegistrationDisabled indicates that a new external identity may not create an account.
	ErrRegistrationDisabled = errors.New("user registration is disabled")
	// ErrIdentityApprovalRequired indicates that an OIDC identity awaits administrator approval.
	ErrIdentityApprovalRequired = errors.New("process OIDC identity: administrator approval is required")
	// ErrIdentityRejected indicates that an administrator rejected an OIDC identity.
	ErrIdentityRejected = errors.New("process OIDC identity: identity was rejected")
	// ErrPageInBin indicates that a page path is occupied by a recycled page.
	ErrPageInBin = errors.New("page path is in recycle bin")
	// ErrStaleReview indicates that a page changed after review was requested.
	ErrStaleReview = errors.New("page changed after review was requested")
	// ErrReviewPending indicates that a page already has a pending review request.
	ErrReviewPending = errors.New("review already pending")
	// ErrReviewClosed indicates that a completed review request cannot be changed.
	ErrReviewClosed = errors.New("review request is closed")
	// ErrReviewChangesRequired indicates that reviewer feedback must be addressed before another request.
	ErrReviewChangesRequired = errors.New("review changes must be addressed")
)

const (
	// PageReviewStatusPending means the requested revision is awaiting a decision.
	PageReviewStatusPending = "pending"
	// PageReviewStatusChangesRequested means a reviewer asked the author to update the page.
	PageReviewStatusChangesRequested = "changes_requested"
	// PageReviewStatusApproved means the requested revision was approved.
	PageReviewStatusApproved = "approved"
	// PageReviewStatusCanceled means the requester or an administrator canceled the request.
	PageReviewStatusCanceled = "canceled"
	// PageReviewStatusSuperseded means a newer page revision replaced the requested revision.
	PageReviewStatusSuperseded = "superseded"
	// PageWatchScopePage subscribes to changes on one exact page.
	PageWatchScopePage = "page"
	// PageWatchScopeSubtree subscribes to the selected page path and descendants.
	PageWatchScopeSubtree = "subtree"
	// NavigationStyleSidebar keeps Kumbuka's full navigation sidebar.
	NavigationStyleSidebar = "sidebar"
	// NavigationStyleTopbar moves page navigation into a horizontal desktop bar.
	NavigationStyleTopbar = "topbar"
	// NavigationStyleTree uses a focused page-tree sidebar.
	NavigationStyleTree = "tree"
	// NavigationDensityComfortable is the default roomy sidebar layout.
	NavigationDensityComfortable = "comfortable"
	// NavigationDensityCompact reduces vertical navigation spacing.
	NavigationDensityCompact = "compact"
	// RobotsPolicyAllow permits crawlers to crawl the application.
	RobotsPolicyAllow = "allow"
	// RobotsPolicyDisallow asks crawlers not to crawl the application.
	RobotsPolicyDisallow = "disallow"
	// RobotsPolicyNone disables robots.txt output.
	RobotsPolicyNone = "none"
	// TypographySizeCompact uses the smallest content typography preset.
	TypographySizeCompact = "compact"
	// TypographySizeStandard uses the regular content typography preset.
	TypographySizeStandard = "standard"
	// TypographySizeLarge uses the largest content typography preset.
	TypographySizeLarge = "large"
	// DefaultTypographySize is the application default on new installations.
	DefaultTypographySize = TypographySizeCompact
	// DefaultSidebarWidth is the default desktop sidebar width in CSS pixels.
	DefaultSidebarWidth = 280
	// MinSidebarWidth is the smallest supported desktop sidebar width.
	MinSidebarWidth = 220
	// MaxSidebarWidth is the largest supported desktop sidebar width.
	MaxSidebarWidth = 420
)

// AdminStats contains high-level object counts shown on the administration page.
type AdminStats struct {
	// Users is the number of wiki users.
	Users int64
	// Groups is the number of user groups.
	Groups int64
	// Pages is the number of active wiki pages.
	Pages int64
	// DeletedPages is the number of pages currently in the recycle bin.
	DeletedPages int64
	// Tags is the number of tags.
	Tags int64
	// Images is the number of uploaded images.
	Images int64
	// Tokens is the number of active or expired API tokens.
	Tokens int64
}

// AdminUser contains a user and the groups currently assigned to the account.
type AdminUser struct {
	// User is the wiki account.
	User User
	// HasLocalCredential reports whether the account has a local password.
	HasLocalCredential bool
	// LocalCredentialEnabled reports whether that local password can be used to sign in.
	LocalCredentialEnabled bool
	// ExternalAdminObserved reports whether an external provider supplied admin status for this account.
	ExternalAdminObserved bool
	// ExternalAdmin reports the most recently observed external administrator status.
	ExternalAdmin bool
	// Groups contains group names assigned to the user.
	Groups []string
	// OIDCIdentities contains external OIDC identities bound to the user.
	OIDCIdentities []OIDCIdentity
	// LastLogin is the most recent successful authentication time.
	LastLogin time.Time
	// HasLoggedIn reports whether LastLogin represents an actual login.
	HasLoggedIn bool
}

// Group describes one administratively managed user group.
type Group struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Name is the unique group name.
	Name string `json:"name"`
	// UserCount is the number of users assigned to the group.
	UserCount int64 `json:"user_count,omitempty"`
	// PageCount is the number of pages assigned to the group.
	PageCount int64 `json:"page_count,omitempty"`
}

// TagInfo describes one tag and its current page usage count.
type TagInfo struct {
	// ID is the stable identifier.
	ID int64
	// Name is the normalized tag name.
	Name string
	// PageCount is the number of pages using the tag.
	PageCount int64
}

// RenderingSettings contains application-wide content presentation defaults.
type RenderingSettings struct {
	// DefaultTypographySize is used when a user has not selected a personal content size.
	DefaultTypographySize string
}

// AuthenticationSettings controls browser authentication without storing secrets.
type AuthenticationSettings struct {
	// Mode selects local, trusted-proxy, or OIDC authentication.
	Mode string
	// OIDCIssuer is the OIDC discovery issuer URL.
	OIDCIssuer string
	// OIDCClientID is the public OIDC client identifier.
	OIDCClientID string
	// OIDCGroupClaim is the top-level claim containing external group names.
	OIDCGroupClaim string
	// OIDCGroupSync enables synchronization of explicitly mapped OIDC groups.
	OIDCGroupSync bool
	// OIDCGroupsAuthoritative removes mapped memberships that disappear from the claim.
	OIDCGroupsAuthoritative bool
	// OIDCGroupMappings maps external group values to Kumbuka groups.
	OIDCGroupMappings []OIDCGroupMapping
	// OIDCAdminGroup is the claim value that grants session-scoped administrator access.
	OIDCAdminGroup string
	// TrustedUsernameHeaders lists trusted-proxy username headers in priority order.
	TrustedUsernameHeaders []string
	// TrustedEmailHeaders lists trusted-proxy email headers in priority order.
	TrustedEmailHeaders []string
	// TrustedDisplayNameHeaders lists trusted-proxy display-name headers in priority order.
	TrustedDisplayNameHeaders []string
	// TrustedGroupHeaders lists trusted-proxy group header candidates.
	TrustedGroupHeaders []string
	// TrustedAdminGroup is the trusted group value that grants administrator access.
	TrustedAdminGroup string
}

// ExternalLink describes one configurable top-bar link.
type ExternalLink struct {
	Label       string `json:"label" toml:"label"`
	URL         string `json:"url" toml:"url"`
	Icon        string `json:"icon,omitempty" toml:"icon"`
	Description string `json:"description,omitempty" toml:"description"`
	// HoverEffect controls visual feedback when a pointer hovers over the link.
	HoverEffect string `json:"hover_effect,omitempty" toml:"hover_effect"`
	// HoverText is an optional title template supporting {{label}} and {{description}}.
	HoverText string `json:"hover_text,omitempty" toml:"hover_text"`
}

const (
	// ExternalLinkHoverHighlight highlights a link without moving it.
	ExternalLinkHoverHighlight = "highlight"
	// ExternalLinkHoverLift raises a link slightly on pointer hover.
	ExternalLinkHoverLift = "lift"
	// ExternalLinkHoverNone disables visual hover treatment.
	ExternalLinkHoverNone = "none"
)

// ExternalLinkHoverEffects returns the supported external-link hover presentations.
func ExternalLinkHoverEffects() []string {
	return []string{ExternalLinkHoverHighlight, ExternalLinkHoverLift, ExternalLinkHoverNone}
}

// ValidExternalLinkHoverEffect reports whether value is a supported hover presentation.
// Empty selects the default highlight presentation.
func ValidExternalLinkHoverEffect(value string) bool {
	switch value {
	case "", ExternalLinkHoverHighlight, ExternalLinkHoverLift, ExternalLinkHoverNone:
		return true
	default:
		return false
	}
}

// EffectiveExternalLinkHoverEffect returns the visual hover presentation used for a link.
func EffectiveExternalLinkHoverEffect(value string) string {
	if value == "" {
		return ExternalLinkHoverHighlight
	}
	return value
}

// ExternalLinkHoverTitle resolves a link's hover text template.
func ExternalLinkHoverTitle(link ExternalLink) string {
	label := strings.TrimSpace(link.Label)
	description := strings.TrimSpace(link.Description)
	template := strings.TrimSpace(link.HoverText)

	if template == "" && description == "" {
		return label
	}
	if template == "" {
		return label + " — " + description
	}

	return strings.TrimSpace(expandExternalLinkHoverTemplate(template, link))
}

// expandExternalLinkHoverTemplate replaces supported placeholders and preserves unknown ones.
func expandExternalLinkHoverTemplate(value string, link ExternalLink) string {
	var output strings.Builder

	for value != "" {
		start := strings.Index(value, "{{")
		if start < 0 {
			output.WriteString(value)
			break
		}

		output.WriteString(value[:start])

		rest := value[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			output.WriteString(value[start:])
			break
		}

		placeholderEnd := start + 2 + end
		name := strings.TrimSpace(value[start+2 : placeholderEnd])

		switch name {
		case "label":
			output.WriteString(link.Label)
		case "description":
			output.WriteString(link.Description)
		default:
			output.WriteString(value[start : placeholderEnd+2])
		}

		value = value[placeholderEnd+2:]
	}

	return output.String()
}

// PDFHeader describes one configurable request header sent to the external PDF service.
type PDFHeader struct {
	ID        int64
	Name      string
	Value     string
	Sensitive bool
	// Configured reports whether a sensitive header has a stored value without exposing it.
	Configured bool
}

// ApplicationSettings contains mutable application-wide settings.
type ApplicationSettings struct {
	// AllowUserRegistration permits new OIDC and trusted-proxy identities to create wiki accounts.
	AllowUserRegistration bool
	// DiscussionsEnabled enables page comments and anchored discussions.
	DiscussionsEnabled bool
	// ContentLanguage is the BCP 47 language tag applied to wiki content and the editor.
	ContentLanguage string
	// PDFURL is the persisted HTML-to-PDF rendering endpoint.
	PDFURL string
	// ExternalLinks contains configurable links rendered beside global search.
	ExternalLinks []ExternalLink
	// RobotsPolicy controls whether robots.txt allows, disallows, or omits crawler guidance.
	RobotsPolicy string
	// Authentication contains non-secret browser authentication settings.
	Authentication AuthenticationSettings
	// Rendering contains application-wide content presentation defaults.
	Rendering RenderingSettings
}

// Attachment contains metadata for a stored non-image file.
type Attachment struct {
	ID          int64     `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	SizeBytes   int64     `json:"size_bytes"`
	UploadedBy  int64     `json:"uploaded_by"`
	Uploader    string    `json:"uploader"`
	CreatedAt   time.Time `json:"created_at"`
	UsageCount  int64     `json:"usage_count"`
}

// AttachmentData contains stored attachment bytes.
type AttachmentData struct {
	Attachment
	Data []byte
}

// AuditEvent describes one administratively visible application action.
type AuditEvent struct {
	ID         int64
	Actor      string
	Action     string
	ObjectType string
	ObjectKey  string
	Detail     string
	CreatedAt  time.Time
}

// BrokenWikiLink describes a wiki link whose target page does not exist.
type BrokenWikiLink struct {
	SourceSlug  string
	SourceTitle string
	TargetSlug  string
}

// DocumentationHealth groups page-quality findings for administrators.
type DocumentationHealth struct {
	BrokenLinks   []BrokenWikiLink
	OrphanPages   []Page
	UntaggedPages []Page
	UniconedPages []Page
	StalePages    []Page
	ReviewDue     []Page
	DraftPages    []Page
	Deprecated    []Page
}

// Image contains metadata for one uploaded wiki image.
type Image struct {
	// ID is the stable identifier used in image URLs.
	ID int64 `json:"id"`
	// Filename is the sanitized original image filename.
	Filename string `json:"filename"`
	// ContentType is the validated image MIME type.
	ContentType string `json:"content_type"`
	// SizeBytes is the stored image size in bytes.
	SizeBytes int64 `json:"size_bytes"`
	// UploadedBy is the identifier of the user that uploaded the image.
	UploadedBy int64 `json:"uploaded_by"`
	// Uploader is the display name of the user that uploaded the image.
	Uploader string `json:"uploader"`
	// CreatedAt is the upload timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UsageCount is the number of Markdown references to the image across all pages.
	UsageCount int64 `json:"usage_count"`
}

// ImageData contains the binary payload and response metadata for one image.
type ImageData struct {
	// Filename is the sanitized image filename.
	Filename string
	// ContentType is the validated image MIME type.
	ContentType string
	// Data is the complete stored image payload.
	Data []byte
}

// PageMetadata contains optional workflow metadata attached to a page.
type PageMetadata struct {
	Status             string             `json:"status"`
	OwnerGroupID       int64              `json:"owner_group_id,omitempty"`
	ReviewIntervalDays int                `json:"review_interval_days,omitempty"`
	MarkReviewed       bool               `json:"mark_reviewed,omitempty"`
	DeprecatedTarget   string             `json:"deprecated_target,omitempty"`
	PluginUsage        *pluginusage.Index `json:"-"`
}

// PageProperty is one searchable structured metadata value attached to a page.
type PageProperty struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// SavedSearch is a named user search that can be surfaced in navigation.
type SavedSearch struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Query  string `json:"query"`
	Pinned bool   `json:"pinned"`
}

// PageWatch is one user's subscription to a page path.
type PageWatch struct {
	Path      string    `json:"path"`
	Scope     string    `json:"scope"`
	CreatedAt time.Time `json:"created_at"`
}

// PageReviewRequest tracks lightweight documentation approval for one revision.
type PageReviewRequest struct {
	ID                int64
	PageSlug          string
	RevisionNumber    int
	RequestedBy       int64
	RequestedByName   string
	ReviewerGroupID   int64
	ReviewerGroupName string
	Reviewers         []User
	ReviewedBy        int64
	ReviewedByName    string
	Status            string
	Note              string
	DecisionNote      string
	PreviousStatus    string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// PageAccess is the effective nearest path-rule decision for one user.
type PageAccess struct {
	Restricted bool
	CanView    bool
	CanEdit    bool
}

// PageAccessRule grants one group view or edit access at a path.
type PageAccessRule struct {
	ID        int64
	Path      string
	GroupID   int64
	GroupName string
	Access    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// WebhookHeader is one configurable HTTP header sent with a webhook request.
type WebhookHeader struct {
	ID        int64
	Name      string
	Value     string `json:"-"`
	Sensitive bool
	// Configured reports whether a sensitive header has a stored value without exposing it.
	Configured bool
}

// Webhook is one administrator-configured outgoing event destination.
type Webhook struct {
	ID              int64
	Name            string
	URL             string
	Events          []string
	BodyTemplate    string
	Headers         []WebhookHeader
	RetryEnabled    bool
	RetryCount      int
	RetryBackoff    time.Duration
	RetryMaxBackoff time.Duration
	RetryJitter     bool
	Enabled         bool
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// WebhookDelivery records the latest outcome of delivering an outgoing event.
type WebhookDelivery struct {
	ID          int64
	WebhookID   int64
	WebhookName string
	Event       string
	StatusCode  int
	Attempts    int
	Error       string
	CreatedAt   time.Time
}

// Notification is a lightweight user inbox item.
type Notification struct {
	ID        int64      `json:"id"`
	Kind      string     `json:"kind"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	URL       string     `json:"url"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// PageComment is a discussion item optionally anchored to selected page text.
type PageComment struct {
	ID        int64      `json:"id"`
	PageID    int64      `json:"page_id"`
	Author    string     `json:"author"`
	Anchor    string     `json:"anchor"`
	Body      string     `json:"body"`
	Resolved  *time.Time `json:"resolved_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// GraphNode is one page in the wiki relationship graph.
type GraphNode struct {
	Slug   string `json:"slug"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// GraphEdge is one wiki-link relationship between pages.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// KnowledgeGraph contains graph nodes and link edges.
type KnowledgeGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// RecentEdit describes a page recently edited by one user.
type RecentEdit struct {
	Page
	RevisionMessage string
}

// PageDraft is a private, autosaved editor state owned by one user.
type PageDraft struct {
	ID              int64               `json:"id"`
	Key             string              `json:"key"`
	PageID          int64               `json:"page_id,omitempty"`
	BaseRevision    int                 `json:"base_revision"`
	CurrentRevision int                 `json:"current_revision"`
	Stale           bool                `json:"stale"`
	Title           string              `json:"title"`
	Slug            string              `json:"slug"`
	PageSlug        string              `json:"page_slug,omitempty"`
	Values          map[string][]string `json:"values,omitempty"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// MovePageOptions controls safe page-tree refactoring.
type MovePageOptions struct {
	MoveChildren        bool
	UpdateIncomingLinks bool
	KeepAliases         bool
}

// NavigationItem describes one page or synthetic folder in the navigation tree.
type NavigationItem struct {
	// Path is the complete navigation path.
	Path string
	// Title is the page title or the raw path segment for synthetic folders.
	Title string
	// Icon is the explicitly selected icon identifier.
	Icon string
	// Page reports whether the path maps to a real wiki page.
	Page bool
}

// OIDCIdentity binds one Kumbuka account to a stable identity from an OIDC issuer.
type OIDCIdentity struct {
	UserID    int64
	Issuer    string
	Subject   string
	CreatedAt time.Time
}

// OIDCGroupMapping maps one external OIDC group value to a Kumbuka group.
type OIDCGroupMapping struct {
	OIDCGroup string
	GroupID   int64
	GroupName string
}

// PendingOIDCIdentity is a verified but not yet accepted OIDC identity.
type PendingOIDCIdentity struct {
	ID                   int64
	Issuer               string
	Subject              string
	Username             string
	Email                string
	DisplayName          string
	Status               string
	FirstSeenAt          time.Time
	LastSeenAt           time.Time
	SuggestedUserID      int64
	SuggestedUsername    string
	SuggestedDisplayName string
}

// PageLink describes one wiki link recorded for a source page.
type PageLink struct {
	TargetSlug  string
	TargetTitle string
	Exists      bool
}

// PageTemplateField is one author-supplied value used by a page blueprint.
type PageTemplateField struct {
	Name     string `json:"name"`
	Label    string `json:"label"`
	Default  string `json:"default,omitempty"`
	Required bool   `json:"required,omitempty"`
}

// PageTemplate is a reusable page blueprint offered when creating a page.
type PageTemplate struct {
	ID                 int64
	Name               string
	Description        string
	Markdown           string
	PathPrefix         string
	Icon               string
	Tags               []string
	Status             string
	OwnerGroupID       int64
	ReviewIntervalDays int
	Properties         map[string]string
	Fields             []PageTemplateField
}

// UserPreferences contains presentation preferences for one wiki user.
type UserPreferences struct {
	// Theme is the filename-derived title of the user's selected theme.
	Theme string
	// ShowPageContents controls whether wiki pages render a heading table of contents.
	ShowPageContents bool
	// NavigationStyle controls the desktop navigation layout.
	NavigationStyle string
	// NavigationDensity controls vertical spacing in the page tree.
	NavigationDensity string
	// TypographySize overrides the application content size; empty inherits the administrator default.
	TypographySize string
	// SidebarWidth is the desktop sidebar width in CSS pixels.
	SidebarWidth int
	// ShowNavigationGuides controls tree indentation guide lines.
	ShowNavigationGuides bool
	// RememberNavigationState persists expanded and collapsed navigation folders.
	RememberNavigationState bool
	// HiddenPluginWidgets contains plugin/module keys hidden by this user.
	HiddenPluginWidgets []string
	// ShowNavigationPageCounts displays descendant page counts for folders.
	ShowNavigationPageCounts bool
	// ExpandedNavigation contains folder slugs the user explicitly left expanded.
	ExpandedNavigation []string
}

// PageShareLink identifies the page exposed by one public permalink.
type PageShareLink struct {
	PageID int64
	Slug   string
	Title  string
}

// User represents an authenticated wiki account.
type User struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Username is the unique login name.
	Username string `json:"username"`
	// Email is the account email address.
	Email string `json:"email"`
	// DisplayName is the human-readable account name.
	DisplayName string `json:"display_name"`
	// Role controls the account authorization level.
	Role string `json:"role"`
	// Enabled controls authentication through every method.
	Enabled bool `json:"-"`
	// ExternalAdmin is an authentication-time administrator elevation.
	ExternalAdmin bool `json:"-"`
	// SessionVersion invalidates previously issued external browser sessions when incremented.
	SessionVersion int64 `json:"-"`
}

// PageHeading is one persisted heading in a rendered page artifact.
type PageHeading struct {
	// Level is the HTML heading level from 1 through 6.
	Level int `json:"level"`
	// ID is the rendered heading anchor identifier.
	ID string `json:"id"`
	// Title is the plain-text heading label.
	Title string `json:"title"`
}

// PageRender is a reusable, theme-independent render artifact for one current page revision.
// Empty Fingerprint means the page must be rendered dynamically.
type PageRender struct {
	// HTML is sanitized rendered Markdown.
	HTML string `json:"-"`
	// Contents contains persisted headings in document order.
	Contents []PageHeading `json:"-"`
	// Fingerprint identifies the renderer/plugins/settings that produced the artifact.
	Fingerprint string `json:"-"`
}

// Page represents the current state of a wiki page.
type Page struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Slug is the unique URL path for the page.
	Slug string `json:"slug"`
	// Title is the human-readable page title.
	Title string `json:"title"`
	// Icon is the optional icon displayed with the page title.
	Icon string `json:"icon,omitempty"`
	// Language optionally overrides the wiki-wide content language.
	Language string `json:"language,omitempty"`
	// Markdown is the current Markdown body.
	Markdown string `json:"markdown_content,omitempty"`
	// PluginUsage is rebuildable derived metadata for source-aware rendering modules.
	PluginUsage *pluginusage.Index `json:"-"`
	// Render contains the reusable current-revision HTML artifact when one is safe to persist.
	Render PageRender `json:"-"`
	// CreatedBy is the identifier of the user that created the page.
	CreatedBy int64 `json:"created_by"`
	// UpdatedBy is the identifier of the user that last updated the page.
	UpdatedBy int64 `json:"updated_by"`
	// Author is the display name of the last editor.
	Author string `json:"author"`
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt is the most recent update timestamp.
	UpdatedAt time.Time `json:"updated_at"`
	// Tags contains normalized page tags.
	Tags []string `json:"tags"`
	// Groups contains the collaboration groups responsible for the page.
	Groups []Group `json:"groups,omitempty"`
	// ViewCount is the total recorded page views.
	ViewCount int64 `json:"view_count"`
	// Status is the page lifecycle state.
	Status string `json:"status"`
	// OwnerGroupID optionally assigns documentation ownership to a collaboration group.
	OwnerGroupID int64 `json:"owner_group_id,omitempty"`
	// OwnerGroup is the human-readable owner group name.
	OwnerGroup string `json:"owner_group,omitempty"`
	// LastReviewedAt records the most recent explicit documentation review.
	LastReviewedAt *time.Time `json:"last_reviewed_at,omitempty"`
	// ReviewIntervalDays configures when the page should be reviewed again.
	ReviewIntervalDays int `json:"review_interval_days,omitempty"`
	// DeprecatedTarget points readers of a deprecated page to its replacement.
	DeprecatedTarget string `json:"deprecated_target,omitempty"`
	// Properties contains searchable structured page metadata.
	Properties []PageProperty `json:"properties,omitempty"`
	// Rank is the search relevance score.
	Rank float32 `json:"rank,omitempty"`
}

// DeletedPage describes a page currently held in the administrator recycle bin.
type DeletedPage struct {
	Page
	DeletedAt time.Time
	DeletedBy string
}

// APIToken contains non-secret metadata for one personal access token.
type APIToken struct {
	// ID is the stable identifier.
	ID int64 `json:"id"`
	// Name is the user-supplied token label.
	Name string `json:"name"`
	// UserID is the account authenticated by the token.
	UserID int64 `json:"user_id"`
	// Username is the login name authenticated by the token.
	Username string `json:"username"`
	// CreatedBy is the account that issued the token.
	CreatedBy int64 `json:"created_by"`
	// Creator is the display name of the account that issued the token.
	Creator string `json:"creator"`
	// CreatedAt is the issuance timestamp.
	CreatedAt time.Time `json:"created_at"`
	// LastUsed is the most recent successful authentication timestamp.
	LastUsed *time.Time `json:"last_used,omitempty"`
	// ExpiresAt is the optional expiration timestamp.
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// IssuedToken contains token metadata and the one-time plaintext secret.
type IssuedToken struct {
	// Token contains the non-secret token metadata.
	Token APIToken `json:"token"`
	// Secret is the plaintext token shown only immediately after creation.
	Secret string `json:"secret"`
}

// ValidUserRole reports whether value is a supported account role.
func ValidUserRole(value string) bool {
	switch value {
	case "admin", "editor", "viewer":
		return true
	default:
		return false
	}
}

// ValidNavigationStyle reports whether value is a supported desktop navigation layout.
func ValidNavigationStyle(value string) bool {
	switch value {
	case NavigationStyleSidebar, NavigationStyleTopbar, NavigationStyleTree:
		return true
	default:
		return false
	}
}

// ValidNavigationDensity reports whether value is a supported navigation density.
func ValidNavigationDensity(value string) bool {
	return value == NavigationDensityComfortable || value == NavigationDensityCompact
}

// ValidRobotsPolicy reports whether value is a supported robots.txt policy.
func ValidRobotsPolicy(value string) bool {
	switch value {
	case RobotsPolicyAllow, RobotsPolicyDisallow, RobotsPolicyNone:
		return true
	default:
		return false
	}
}

// ValidTypographySize reports whether value is a supported content typography preset.
func ValidTypographySize(value string) bool {
	switch value {
	case TypographySizeCompact, TypographySizeStandard, TypographySizeLarge:
		return true
	default:
		return false
	}
}

// ValidSidebarWidth reports whether width is inside the supported desktop range.
func ValidSidebarWidth(width int) bool {
	return width >= MinSidebarWidth && width <= MaxSidebarWidth
}

// ValidReviewIntervalDays reports whether days is a supported review interval.
func ValidReviewIntervalDays(days int) bool {
	return days >= 0 && days <= 3650
}

// PageStatuses returns the supported page lifecycle statuses.
func PageStatuses() []string {
	return []string{"draft", "verified", "deprecated", "archived"}
}

// ValidPageStatus reports whether a page status is supported.
func ValidPageStatus(value string) bool {
	return slices.Contains(PageStatuses(), value)
}

// DefaultUserPreferences returns the presentation defaults used before a user saves preferences.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		ShowPageContents:         true,
		NavigationStyle:          NavigationStyleSidebar,
		NavigationDensity:        NavigationDensityComfortable,
		SidebarWidth:             DefaultSidebarWidth,
		ShowNavigationGuides:     true,
		RememberNavigationState:  true,
		ShowNavigationPageCounts: false,
		ExpandedNavigation:       []string{},
		HiddenPluginWidgets:      []string{},
	}
}
