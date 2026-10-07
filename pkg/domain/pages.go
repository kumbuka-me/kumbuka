package domain

import (
	"slices"
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// PageHeading is one persisted heading in a rendered page artifact.
type PageHeading struct {
	// Level is the HTML heading level from 1 through 6.
	Level int `json:"level"`
	// ID is the rendered heading anchor identifier.
	ID string `json:"id"`
	// Title is the plain-text heading label.
	Title string `json:"title"`
}

// PageRender is a reusable, theme-independent render artifact for one current page revision. Empty Fingerprint means the page must be rendered dynamically.
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
	Status PageStatus `json:"status"`
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
	// DeletedBy is the display name of the user who deleted the page.
	DeletedBy string
}

// PageStatuses returns the supported page lifecycle statuses.
func PageStatuses() []string {
	return []string{string(PageStatusDraft), string(PageStatusVerified), string(PageStatusDeprecated), string(PageStatusArchived)}
}

// ValidPageStatus reports whether a page status is supported.
func ValidPageStatus(value PageStatus) bool {
	return slices.Contains([]PageStatus{PageStatusDraft, PageStatusVerified, PageStatusDeprecated, PageStatusArchived}, value)
}
