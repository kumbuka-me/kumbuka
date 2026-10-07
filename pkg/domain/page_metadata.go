package domain

import (
	"time"

	"github.com/kumbuka-me/kumbuka/pkg/pluginusage"
)

// PageMetadata contains optional workflow metadata attached to a page.
type PageMetadata struct {
	// Status is the current status of page metadata.
	Status PageStatus `json:"status"`
	// OwnerGroupID identifies the owner group associated with page metadata.
	OwnerGroupID int64 `json:"owner_group_id,omitempty"`
	// ReviewIntervalDays controls how often the page is due for review.
	ReviewIntervalDays int `json:"review_interval_days,omitempty"`
	// MarkReviewed records the current save as an explicit review.
	MarkReviewed bool `json:"mark_reviewed,omitempty"`
	// DeprecatedTarget is the replacement path for a deprecated page.
	DeprecatedTarget string `json:"deprecated_target,omitempty"`
	// PluginUsage is derived metadata for source-aware rendering modules.
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
	// Query is the search expression saved by the user.
	Query string `json:"query"`
	// Pinned keeps the saved search visible in navigation.
	Pinned bool `json:"pinned"`
}

// PageWatch is one user's subscription to a page path.
type PageWatch struct {
	// Path is the path associated with page watch.
	Path string `json:"path"`
	// Scope controls which changes trigger the watch.
	Scope PageWatchScope `json:"scope"`
	// CreatedAt records the created at timestamp for page watch.
	CreatedAt time.Time `json:"created_at"`
}
