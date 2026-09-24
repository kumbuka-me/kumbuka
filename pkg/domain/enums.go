package domain

// PageReviewStatus identifies the lifecycle state of a page review request.
type PageReviewStatus string

const (
	// PageReviewStatusPending means the requested revision is awaiting a decision.
	PageReviewStatusPending PageReviewStatus = "pending"
	// PageReviewStatusChangesRequested means a reviewer asked the author to update the page.
	PageReviewStatusChangesRequested PageReviewStatus = "changes_requested"
	// PageReviewStatusApproved means the requested revision was approved.
	PageReviewStatusApproved PageReviewStatus = "approved"
	// PageReviewStatusCanceled means the requester or an administrator canceled the request.
	PageReviewStatusCanceled PageReviewStatus = "canceled"
	// PageReviewStatusSuperseded means a newer page revision replaced the requested revision.
	PageReviewStatusSuperseded PageReviewStatus = "superseded"
)

// PageReviewCommentSide identifies the side of a revision diff referenced by review feedback.
type PageReviewCommentSide string

const (
	// PageReviewCommentSideOld anchors feedback to a removed line from the previous revision.
	PageReviewCommentSideOld PageReviewCommentSide = "old"
	// PageReviewCommentSideNew anchors feedback to a line in the reviewed revision.
	PageReviewCommentSideNew PageReviewCommentSide = "new"
)

// PageWatchScope identifies how broadly a page watch applies.
type PageWatchScope string

const (
	// PageWatchScopePage subscribes to changes on one exact page.
	PageWatchScopePage PageWatchScope = "page"
	// PageWatchScopeSubtree subscribes to the selected page path and descendants.
	PageWatchScopeSubtree PageWatchScope = "subtree"
)

// NavigationStyle identifies a supported desktop navigation layout.
type NavigationStyle string

const (
	// NavigationStyleSidebar keeps Kumbuka's full navigation sidebar.
	NavigationStyleSidebar NavigationStyle = "sidebar"
	// NavigationStyleTopbar moves page navigation into a horizontal desktop bar.
	NavigationStyleTopbar NavigationStyle = "topbar"
	// NavigationStyleTree uses a focused page-tree sidebar.
	NavigationStyleTree NavigationStyle = "tree"
)

// NavigationDensity identifies a supported navigation spacing preset.
type NavigationDensity string

const (
	// NavigationDensityComfortable is the default roomy sidebar layout.
	NavigationDensityComfortable NavigationDensity = "comfortable"
	// NavigationDensityCompact reduces vertical navigation spacing.
	NavigationDensityCompact NavigationDensity = "compact"
)

// RobotsPolicy identifies the robots.txt behavior selected by an administrator.
type RobotsPolicy string

const (
	// RobotsPolicyAllow permits crawlers to crawl the application.
	RobotsPolicyAllow RobotsPolicy = "allow"
	// RobotsPolicyDisallow asks crawlers not to crawl the application.
	RobotsPolicyDisallow RobotsPolicy = "disallow"
	// RobotsPolicyNone disables robots.txt output.
	RobotsPolicyNone RobotsPolicy = "none"
)

// TypographySize identifies a supported content typography preset.
type TypographySize string

const (
	// TypographySizeCompact uses the smallest content typography preset.
	TypographySizeCompact TypographySize = "compact"
	// TypographySizeStandard uses the regular content typography preset.
	TypographySizeStandard TypographySize = "standard"
	// TypographySizeLarge uses the largest content typography preset.
	TypographySizeLarge TypographySize = "large"
	// DefaultTypographySize is the application default on new installations.
	DefaultTypographySize TypographySize = TypographySizeCompact
)

// ExternalLinkHoverEffect identifies the visual hover treatment for an external link.
type ExternalLinkHoverEffect string

const (
	// ExternalLinkHoverHighlight highlights a link without moving it.
	ExternalLinkHoverHighlight ExternalLinkHoverEffect = "highlight"
	// ExternalLinkHoverLift raises a link slightly on pointer hover.
	ExternalLinkHoverLift ExternalLinkHoverEffect = "lift"
	// ExternalLinkHoverNone disables visual hover treatment.
	ExternalLinkHoverNone ExternalLinkHoverEffect = "none"
)

// PageAccessLevel identifies the access granted to a group for one page path.
type PageAccessLevel string

const (
	// PageAccessView grants read access to a protected page path.
	PageAccessView PageAccessLevel = "view"
	// PageAccessEdit grants read and edit access to a protected page path.
	PageAccessEdit PageAccessLevel = "edit"
)

// PageStatus identifies a page lifecycle state.
type PageStatus string

const (
	// PageStatusDraft identifies work that is not yet verified.
	PageStatusDraft PageStatus = "draft"
	// PageStatusVerified identifies current reviewed documentation.
	PageStatusVerified PageStatus = "verified"
	// PageStatusDeprecated identifies documentation kept for compatibility but no longer preferred.
	PageStatusDeprecated PageStatus = "deprecated"
	// PageStatusArchived identifies documentation retained outside the active lifecycle.
	PageStatusArchived PageStatus = "archived"
)

// PendingOIDCStatus identifies the administrator-review state of a pending OIDC identity.
type PendingOIDCStatus string

const (
	// PendingOIDCStatusPending means the identity still awaits an administrator decision.
	PendingOIDCStatusPending PendingOIDCStatus = "pending"
	// PendingOIDCStatusRejected means an administrator rejected the identity.
	PendingOIDCStatusRejected PendingOIDCStatus = "rejected"
)

// UserRole identifies an account-level authorization role.
type UserRole string

const (
	// UserRoleAdmin grants full account-level administration.
	UserRoleAdmin UserRole = "admin"
	// UserRoleEditor grants account-level content editing without administration.
	UserRoleEditor UserRole = "editor"
	// UserRoleViewer grants authenticated read access without content editing.
	UserRoleViewer UserRole = "viewer"
)

const (
	// DefaultSidebarWidth is the default desktop sidebar width in CSS pixels.
	DefaultSidebarWidth = 280
	// MinSidebarWidth is the smallest supported desktop sidebar width.
	MinSidebarWidth = 220
	// MaxSidebarWidth is the largest supported desktop sidebar width.
	MaxSidebarWidth = 420
)
