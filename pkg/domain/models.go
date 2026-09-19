package domain

import (
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
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
	// ErrReviewSuggestionConflict indicates that selected suggestions overlap and cannot be applied together.
	ErrReviewSuggestionConflict = errors.New("review suggestions overlap")
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
	// PageReviewCommentSideOld anchors feedback to a removed line from the previous revision.
	PageReviewCommentSideOld = "old"
	// PageReviewCommentSideNew anchors feedback to a line in the reviewed revision.
	PageReviewCommentSideNew = "new"
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
	// Label is the display label for external link.
	Label string `json:"label" toml:"label"`
	// URL is the target URL for external link.
	URL string `json:"url" toml:"url"`
	// Icon names the icon used for external link.
	Icon string `json:"icon,omitempty" toml:"icon"`
	// Description describes external link.
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
	// ID identifies PDF header.
	ID int64
	// Name is the name of PDF header.
	Name string
	// Value contains the value represented by PDF header.
	Value string
	// Sensitive reports whether sensitive applies to PDF header.
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
	// ID identifies attachment.
	ID int64 `json:"id"`
	// Filename is the filename associated with attachment.
	Filename string `json:"filename"`
	// ContentType is the content type associated with attachment.
	ContentType string `json:"content_type"`
	// SizeBytes stores the size bytes value used by attachment.
	SizeBytes int64 `json:"size_bytes"`
	// UploadedBy stores the uploaded by value used by attachment.
	UploadedBy int64 `json:"uploaded_by"`
	// Uploader stores the uploader value used by attachment.
	Uploader string `json:"uploader"`
	// CreatedAt records the created at timestamp for attachment.
	CreatedAt time.Time `json:"created_at"`
	// UsageCount is the number of usage associated with attachment.
	UsageCount int64 `json:"usage_count"`
}

// AttachmentData contains stored attachment bytes.
type AttachmentData struct {
	// Attachment embeds attachment behavior in attachment data.
	Attachment
	// Data contains the data associated with attachment data.
	Data []byte
}

// AuditEvent describes one administratively visible application action.
type AuditEvent struct {
	// ID identifies audit event.
	ID int64
	// Actor stores the actor value used by audit event.
	Actor string
	// Action stores the action value used by audit event.
	Action string
	// ObjectType is the object type associated with audit event.
	ObjectType string
	// ObjectKey stores the object key value used by audit event.
	ObjectKey string
	// Detail stores the detail value used by audit event.
	Detail string
	// CreatedAt records the created at timestamp for audit event.
	CreatedAt time.Time
}

// BrokenWikiLink describes a wiki link whose target page does not exist.
type BrokenWikiLink struct {
	// SourceSlug is the source slug associated with broken wiki link.
	SourceSlug string
	// SourceTitle is the source title associated with broken wiki link.
	SourceTitle string
	// TargetSlug is the target slug associated with broken wiki link.
	TargetSlug string
}

// DocumentationHealth groups page-quality findings for administrators.
type DocumentationHealth struct {
	// BrokenLinks contains the broken links associated with documentation health.
	BrokenLinks []BrokenWikiLink
	// OrphanPages contains the orphan pages associated with documentation health.
	OrphanPages []Page
	// UntaggedPages contains the untagged pages associated with documentation health.
	UntaggedPages []Page
	// UniconedPages contains the uniconed pages associated with documentation health.
	UniconedPages []Page
	// StalePages contains the stale pages associated with documentation health.
	StalePages []Page
	// ReviewDue contains the review due associated with documentation health.
	ReviewDue []Page
	// DraftPages contains the draft pages associated with documentation health.
	DraftPages []Page
	// Deprecated contains the deprecated associated with documentation health.
	Deprecated []Page
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
	// Status is the current status of page metadata.
	Status string `json:"status"`
	// OwnerGroupID identifies the owner group associated with page metadata.
	OwnerGroupID int64 `json:"owner_group_id,omitempty"`
	// ReviewIntervalDays stores the review interval days value used by page metadata.
	ReviewIntervalDays int `json:"review_interval_days,omitempty"`
	// MarkReviewed reports whether mark reviewed applies to page metadata.
	MarkReviewed bool `json:"mark_reviewed,omitempty"`
	// DeprecatedTarget stores the deprecated target value used by page metadata.
	DeprecatedTarget string `json:"deprecated_target,omitempty"`
	// PluginUsage stores the plugin usage value used by page metadata.
	PluginUsage *pluginusage.Index `json:"-"`
}

// PageProperty is one searchable structured metadata value attached to a page.
type PageProperty struct {
	// Key is the lookup key for page property.
	Key string `json:"key"`
	// Value contains the value represented by page property.
	Value string `json:"value"`
}

// SavedSearch is a named user search that can be surfaced in navigation.
type SavedSearch struct {
	// ID identifies saved search.
	ID int64 `json:"id"`
	// Name is the name of saved search.
	Name string `json:"name"`
	// Query stores the query value used by saved search.
	Query string `json:"query"`
	// Pinned reports whether pinned applies to saved search.
	Pinned bool `json:"pinned"`
}

// PageWatch is one user's subscription to a page path.
type PageWatch struct {
	// Path is the path associated with page watch.
	Path string `json:"path"`
	// Scope stores the scope value used by page watch.
	Scope string `json:"scope"`
	// CreatedAt records the created at timestamp for page watch.
	CreatedAt time.Time `json:"created_at"`
}

// PageReviewRequest tracks lightweight documentation approval for one revision.
type PageReviewRequest struct {
	// ID identifies page review request.
	ID int64
	// PageSlug is the page slug associated with page review request.
	PageSlug string
	// RevisionNumber stores the revision number value used by page review request.
	RevisionNumber int
	// RequestedBy stores the requested by value used by page review request.
	RequestedBy int64
	// RequestedByName is the requested by name associated with page review request.
	RequestedByName string
	// ReviewerGroupID identifies the reviewer group associated with page review request.
	ReviewerGroupID int64
	// ReviewerGroupName is the reviewer group name associated with page review request.
	ReviewerGroupName string
	// Reviewers contains the reviewers associated with page review request.
	Reviewers []User
	// ReviewedBy stores the reviewed by value used by page review request.
	ReviewedBy int64
	// ReviewedByName is the reviewed by name associated with page review request.
	ReviewedByName string
	// Status is the current status of page review request.
	Status string
	// Note stores the note value used by page review request.
	Note string
	// DecisionNote stores the decision note value used by page review request.
	DecisionNote string
	// PreviousStatus is the previous status associated with page review request.
	PreviousStatus string
	// CreatedAt records the created at timestamp for page review request.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for page review request.
	UpdatedAt time.Time
}

// PageReviewComment is line-anchored feedback attached to one immutable review revision.
type PageReviewComment struct {
	// ID identifies the review comment.
	ID int64
	// ReviewRequestID identifies the review request that owns the comment.
	ReviewRequestID int64
	// AuthorID identifies the user who created the comment.
	AuthorID int64
	// Author is the display name of the comment author.
	Author string
	// Side selects the previous or reviewed side of the revision diff.
	Side string
	// StartLine is the first one-based source line covered by the comment.
	StartLine int
	// EndLine is the last one-based source line covered by the comment.
	EndLine int
	// Body contains the human discussion text associated with the line range.
	Body string
	// IsSuggestion reports whether Replacement proposes an applicable source change.
	IsSuggestion bool
	// Original stores the exact reviewed Markdown range used for conflict detection.
	Original string
	// Replacement stores the Markdown proposed by a suggestion.
	Replacement string
	// AppliedBy identifies the user who applied the suggestion, or zero while unapplied.
	AppliedBy int64
	// AppliedByName is the display name of the user who applied the suggestion.
	AppliedByName string
	// AppliedAt records when a suggestion was applied; nil means it remains unapplied.
	AppliedAt *time.Time
	// CreatedAt records when the review comment was created.
	CreatedAt time.Time
}

// PageAccess is the effective nearest path-rule decision for one user.
type PageAccess struct {
	// Restricted reports whether restricted applies to page access.
	Restricted bool
	// CanView reports whether can view applies to page access.
	CanView bool
	// CanEdit reports whether can edit applies to page access.
	CanEdit bool
}

// PageAccessRule grants one group view or edit access at a path.
type PageAccessRule struct {
	// ID identifies page access rule.
	ID int64
	// Path is the path associated with page access rule.
	Path string
	// GroupID identifies the group associated with page access rule.
	GroupID int64
	// GroupName is the group name associated with page access rule.
	GroupName string
	// Access stores the access value used by page access rule.
	Access string
	// CreatedAt records the created at timestamp for page access rule.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for page access rule.
	UpdatedAt time.Time
}

// WebhookHeader is one configurable HTTP header sent with a webhook request.
type WebhookHeader struct {
	// ID identifies webhook header.
	ID int64
	// Name is the name of webhook header.
	Name string
	// Value contains the value represented by webhook header.
	Value string `json:"-"`
	// Sensitive reports whether sensitive applies to webhook header.
	Sensitive bool
	// Configured reports whether a sensitive header has a stored value without exposing it.
	Configured bool
}

// Webhook is one administrator-configured outgoing event destination.
type Webhook struct {
	// ID identifies webhook.
	ID int64
	// Name is the name of webhook.
	Name string
	// URL is the target URL for webhook.
	URL string
	// Events contains the events associated with webhook.
	Events []string
	// BodyTemplate stores the body template value used by webhook.
	BodyTemplate string
	// Headers contains the headers associated with webhook.
	Headers []WebhookHeader
	// RetryEnabled reports whether retry enabled applies to webhook.
	RetryEnabled bool
	// RetryCount is the number of retry associated with webhook.
	RetryCount int
	// RetryBackoff stores the retry backoff value used by webhook.
	RetryBackoff time.Duration
	// RetryMaxBackoff stores the retry max backoff value used by webhook.
	RetryMaxBackoff time.Duration
	// RetryJitter reports whether retry jitter applies to webhook.
	RetryJitter bool
	// Enabled reports whether enabled applies to webhook.
	Enabled bool
	// CreatedAt records the created at timestamp for webhook.
	CreatedAt time.Time
	// UpdatedAt records the updated at timestamp for webhook.
	UpdatedAt time.Time
}

// WebhookDelivery records the latest outcome of delivering an outgoing event.
type WebhookDelivery struct {
	// ID identifies webhook delivery.
	ID int64
	// WebhookID identifies the webhook associated with webhook delivery.
	WebhookID int64
	// WebhookName is the webhook name associated with webhook delivery.
	WebhookName string
	// Event stores the event value used by webhook delivery.
	Event string
	// StatusCode stores the status code value used by webhook delivery.
	StatusCode int
	// Attempts stores the attempts value used by webhook delivery.
	Attempts int
	// Error stores the error value used by webhook delivery.
	Error string
	// CreatedAt records the created at timestamp for webhook delivery.
	CreatedAt time.Time
}

// PluginRelease describes update metadata needed outside the catalog transport adapter.
type PluginRelease struct {
	// Version is the compatible plugin version offered to administrators.
	Version string
	// ReleasedAt records when the release was published.
	ReleasedAt time.Time
}

// PluginUpdateNotice describes one newly discovered compatible plugin release.
type PluginUpdateNotice struct {
	// ID is the stable plugin identifier.
	ID string
	// Name is the human-readable plugin name.
	Name string
	// CurrentVersion is the version currently active in Kumbuka.
	CurrentVersion string
	// AvailableVersion is the newer compatible catalog version.
	AvailableVersion string
}

// Notification is a lightweight user inbox item.
type Notification struct {
	// ID identifies notification.
	ID int64 `json:"id"`
	// Kind stores the kind value used by notification.
	Kind string `json:"kind"`
	// Title is the title associated with notification.
	Title string `json:"title"`
	// Body stores the body value used by notification.
	Body string `json:"body"`
	// URL is the target URL for notification.
	URL string `json:"url"`
	// ReadAt records the read at timestamp for notification.
	ReadAt *time.Time `json:"read_at,omitempty"`
	// CreatedAt records the created at timestamp for notification.
	CreatedAt time.Time `json:"created_at"`
}

// PageComment is a discussion item optionally anchored to selected page text.
type PageComment struct {
	// ID identifies page comment.
	ID int64 `json:"id"`
	// PageID identifies the page associated with page comment.
	PageID int64 `json:"page_id"`
	// ParentID identifies the comment this item replies to.
	ParentID int64 `json:"parent_id,omitempty"`
	// ParentAuthor is the display name of the replied-to comment author.
	ParentAuthor string `json:"parent_author,omitempty"`
	// ParentBody contains the replied-to comment body for compact context.
	ParentBody string `json:"parent_body,omitempty"`
	// Author stores the author value used by page comment.
	Author string `json:"author"`
	// Anchor stores the anchor value used by page comment.
	Anchor string `json:"anchor"`
	// Quote contains an optional excerpt explicitly quoted by the reply author.
	Quote string `json:"quote,omitempty"`
	// Body stores the body value used by page comment.
	Body string `json:"body"`
	// Resolved stores the resolved value used by page comment.
	Resolved *time.Time `json:"resolved_at,omitempty"`
	// CreatedAt records the created at timestamp for page comment.
	CreatedAt time.Time `json:"created_at"`
}

// GraphNode is one page in the wiki relationship graph.
type GraphNode struct {
	// Slug is the normalized page path associated with graph node.
	Slug string `json:"slug"`
	// Title is the title associated with graph node.
	Title string `json:"title"`
	// Status is the current status of graph node.
	Status string `json:"status"`
}

// GraphEdge is one wiki-link relationship between pages.
type GraphEdge struct {
	// Source records the source associated with graph edge.
	Source string `json:"source"`
	// Target stores the target value used by graph edge.
	Target string `json:"target"`
}

// KnowledgeGraph contains graph nodes and link edges.
type KnowledgeGraph struct {
	// Nodes contains the nodes associated with knowledge graph.
	Nodes []GraphNode `json:"nodes"`
	// Edges contains the edges associated with knowledge graph.
	Edges []GraphEdge `json:"edges"`
}

// RecentEdit describes a page recently edited by one user.
type RecentEdit struct {
	// Page embeds page behavior in recent edit.
	Page
	// RevisionMessage contains the revision message for recent edit.
	RevisionMessage string
}

// PageDraft is a private, autosaved editor state owned by one user.
type PageDraft struct {
	// ID identifies page draft.
	ID int64 `json:"id"`
	// Key is the lookup key for page draft.
	Key string `json:"key"`
	// PageID identifies the page associated with page draft.
	PageID int64 `json:"page_id,omitempty"`
	// BaseRevision stores the base revision value used by page draft.
	BaseRevision int `json:"base_revision"`
	// CurrentRevision stores the current revision value used by page draft.
	CurrentRevision int `json:"current_revision"`
	// Stale reports whether stale applies to page draft.
	Stale bool `json:"stale"`
	// Title is the title associated with page draft.
	Title string `json:"title"`
	// Slug is the normalized page path associated with page draft.
	Slug string `json:"slug"`
	// PageSlug is the page slug associated with page draft.
	PageSlug string `json:"page_slug,omitempty"`
	// Values contains the values represented by page draft.
	Values map[string][]string `json:"values,omitempty"`
	// CreatedAt records the created at timestamp for page draft.
	CreatedAt time.Time `json:"created_at"`
	// UpdatedAt records the updated at timestamp for page draft.
	UpdatedAt time.Time `json:"updated_at"`
}

// MovePageOptions controls safe page-tree refactoring.
type MovePageOptions struct {
	// MoveChildren reports whether move children applies to move page options.
	MoveChildren bool
	// UpdateIncomingLinks reports whether update incoming links applies to move page options.
	UpdateIncomingLinks bool
	// KeepAliases reports whether keep aliases applies to move page options.
	KeepAliases bool
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
	// UserID identifies the user associated with OIDC identity.
	UserID int64
	// Issuer stores the issuer value used by OIDC identity.
	Issuer string
	// Subject stores the subject value used by OIDC identity.
	Subject string
	// CreatedAt records the created at timestamp for OIDC identity.
	CreatedAt time.Time
}

// OIDCGroupMapping maps one external OIDC group value to a Kumbuka group.
type OIDCGroupMapping struct {
	// OIDCGroup stores the OIDC group value used by OIDC group mapping.
	OIDCGroup string
	// GroupID identifies the group associated with OIDC group mapping.
	GroupID int64
	// GroupName is the group name associated with OIDC group mapping.
	GroupName string
}

// PendingOIDCIdentity is a verified but not yet accepted OIDC identity.
type PendingOIDCIdentity struct {
	// ID identifies pending OIDC identity.
	ID int64
	// Issuer stores the issuer value used by pending OIDC identity.
	Issuer string
	// Subject stores the subject value used by pending OIDC identity.
	Subject string
	// Username is the username associated with pending OIDC identity.
	Username string
	// Email stores the email value used by pending OIDC identity.
	Email string
	// DisplayName is the display name associated with pending OIDC identity.
	DisplayName string
	// Status is the current status of pending OIDC identity.
	Status string
	// FirstSeenAt records the first seen at timestamp for pending OIDC identity.
	FirstSeenAt time.Time
	// LastSeenAt records the last seen at timestamp for pending OIDC identity.
	LastSeenAt time.Time
	// SuggestedUserID identifies the suggested user associated with pending OIDC identity.
	SuggestedUserID int64
	// SuggestedUsername is the suggested username associated with pending OIDC identity.
	SuggestedUsername string
	// SuggestedDisplayName is the suggested display name associated with pending OIDC identity.
	SuggestedDisplayName string
}

// PageLink describes one wiki link recorded for a source page.
type PageLink struct {
	// TargetSlug is the target slug associated with page link.
	TargetSlug string
	// TargetTitle is the target title associated with page link.
	TargetTitle string
	// Exists reports whether exists applies to page link.
	Exists bool
}

// PageTemplateField is one author-supplied value used by a page blueprint.
type PageTemplateField struct {
	// Name is the name of page template field.
	Name string `json:"name"`
	// Label is the display label for page template field.
	Label string `json:"label"`
	// Default stores the default value used by page template field.
	Default string `json:"default,omitempty"`
	// Required reports whether required applies to page template field.
	Required bool `json:"required,omitempty"`
}

// PageTemplate is a reusable page blueprint offered when creating a page.
type PageTemplate struct {
	// ID identifies page template.
	ID int64
	// Name is the name of page template.
	Name string
	// Description describes page template.
	Description string
	// Markdown stores the markdown value used by page template.
	Markdown string
	// PathPrefix stores the path prefix value used by page template.
	PathPrefix string
	// Icon names the icon used for page template.
	Icon string
	// Tags contains the tags associated with page template.
	Tags []string
	// Status is the current status of page template.
	Status string
	// OwnerGroupID identifies the owner group associated with page template.
	OwnerGroupID int64
	// ReviewIntervalDays stores the review interval days value used by page template.
	ReviewIntervalDays int
	// Properties maps keys to properties values used by page template.
	Properties map[string]string
	// Fields contains the fields associated with page template.
	Fields []PageTemplateField
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
	// PageID identifies the page associated with page share link.
	PageID int64
	// Slug is the normalized page path associated with page share link.
	Slug string
	// Title is the title associated with page share link.
	Title string
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
	// Page embeds page behavior in deleted page.
	Page
	// DeletedAt records the deleted at timestamp for deleted page.
	DeletedAt time.Time
	// DeletedBy stores the deleted by value used by deleted page.
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
